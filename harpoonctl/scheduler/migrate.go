package scheduler

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
	"github.com/soundcloud/harpoon/harpoon-scheduler/xf"
	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var migrateCommand = cli.Command{
	Name:        "migrate",
	Usage:       "migrate old.json new.json",
	Description: "Interactively migrate an existing job (old.json) to a new configuration (new.json).",
	Action:      migrateAction,
}

func migrateAction(c *cli.Context) {
	if len(c.Args()) != 2 {
		log.Fatalf("usage: migrate old.json new.json")
	}

	var (
		oldFilename = c.Args().Get(0)
		newFilename = c.Args().Get(1)
	)

	oldJobBuf, err := ioutil.ReadFile(oldFilename)
	if err != nil {
		log.Fatalf("%s: %s", oldFilename, err)
	}

	var oldJob configstore.JobConfig
	if err := json.Unmarshal(oldJobBuf, &oldJob); err != nil {
		log.Fatalf("%s: %s", oldFilename, err)
	}

	if err := oldJob.Valid(); err != nil {
		log.Fatalf("%s: %s", oldFilename, err)
	}

	newJobBuf, err := ioutil.ReadFile(newFilename)
	if err != nil {
		log.Fatalf("%s: %s", newFilename, err)
	}

	var newJob configstore.JobConfig
	if err := json.Unmarshal(newJobBuf, &newJob); err != nil {
		log.Fatalf("%s: %s", newFilename, err)
	}

	if err := newJob.Valid(); err != nil {
		log.Fatalf("%s: %s", newFilename, err)
	}

	if oldJob.Job != newJob.Job {
		log.Fatalf("can't migrate different jobs (from %q to %q)", oldJob.Job, newJob.Job)
	}

	// Migrate is a stateful action, so we need to be more careful.

	var (
		signalc    = make(chan os.Signal) // ctrl-C
		interruptc = make(chan struct{})  // into migrate goroutine
		errc       = make(chan error)     // out of migrate goroutine
	)

	signal.Notify(signalc, syscall.SIGINT, syscall.SIGTERM)
	go func() { errc <- migrate(oldJob, newJob, interruptc) }()

	select {
	case sig := <-signalc:
		log.Errorf("received %s, terminating migration...", sig)
		close(interruptc)
		log.Fatalf("migration failed: %s", <-errc)

	case err := <-errc:
		if err != nil {
			log.Fatalf("migration failed: %s", err)
		}
	}

	log.Printf("migration complete")
}

func migrate(oldJob, newJob configstore.JobConfig, interruptc chan struct{}) error {
	abortable := func(f func() error) error {
		errc := make(chan error, 1)

		go func() { errc <- f() }()

		select {
		case <-interruptc:
			return fmt.Errorf("interrupted")
		case err := <-errc:
			return err
		}
	}

	var (
		scheduleTimeout   = newJob.Grace.Startup.Duration * 2
		unscheduleTimeout = oldJob.Grace.Shutdown.Duration * 2
	)

	// Schedule new job.
	if err := abortable(func() error { return schedule(newJob) }); err != nil {
		return fmt.Errorf("request to schedule new job failed: %s", err)
	}

	// Wait for all new tasks to be running.
	if err := abortable(func() error { return waitForRunning(tasksFor(newJob), scheduleTimeout) }); err != nil {
		return fmt.Errorf("when waiting for new tasks: %s", err)
	}

	// Unschedule old job.
	if err := abortable(func() error { return unscheduleConfig(oldJob) }); err != nil {
		return fmt.Errorf("request to unschedule old job failed: %s", err)
	}

	// Wait for all old tasks to be terminated.
	if err := abortable(func() error { return waitForDeleted(tasksFor(oldJob), unscheduleTimeout) }); err != nil {
		return fmt.Errorf("when waiting for old tasks: %s", err)
	}

	return nil
}

func tasksFor(c configstore.JobConfig) []string {
	var (
		hash    = c.Hash()
		taskIDs = []string{}
	)

	for i := 0; i < c.Scale; i++ {
		taskIDs = append(taskIDs, xf.MakeContainerID(hash, i))
	}

	return taskIDs
}

func waitForRunning(taskIDs []string, timeout time.Duration) error {
	var (
		begin    = time.Now()
		deadline = begin.Add(timeout)
		interval = 250 * time.Millisecond
		want     = map[string]struct{}{}
	)

	for _, id := range taskIDs {
		want[id] = struct{}{}
	}

	for _ = range time.Tick(interval) {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout (%s) exceeded", timeout)
		}

		m, err := currentState()
		if err != nil {
			log.Warnf("%s: %s", endpoint.Host, err)
			continue
		}

		have := map[string]struct{}{}

		for _, e := range m {
			for id, ci := range e.Containers {
				if _, ok := want[id]; ok && ci.ContainerStatus == agent.ContainerStatusRunning {
					have[id] = struct{}{}
				}
			}
		}

		if len(have) < len(want) {
			log.Verbosef("waiting for %d task(s) to be started: %d started, %d still pending", len(want), len(have), len(want)-len(have))
			continue
		}

		log.Printf("%d task(s) started in %s", len(taskIDs), time.Since(begin))
		return nil
	}

	panic("unreachable")
}

func waitForDeleted(taskIDs []string, timeout time.Duration) error {
	var (
		begin    = time.Now()
		deadline = begin.Add(timeout)
		interval = 250 * time.Millisecond
		want     = map[string]struct{}{}
	)

	for _, id := range taskIDs {
		want[id] = struct{}{}
	}

	for _ = range time.Tick(interval) {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout (%s) exceeded", timeout)
		}

		m, err := currentState()
		if err != nil {
			log.Warnf("%s: %s", endpoint.Host, err)
			continue
		}

		for wantID := range want {
			if found := func() bool {
				for _, e := range m {
					for foundID := range e.Containers {
						if wantID == foundID {
							return true
						}
					}
				}
				return false
			}(); found {
				delete(want, wantID)
			}
		}

		if len(want) > 0 {
			log.Verbosef("waiting for %d task(s) to be stopped: %d stopped, %d still pending", len(taskIDs), len(taskIDs)-len(want), len(want))
			continue
		}

		log.Printf("%d task(s) stopped in %s", len(taskIDs), time.Since(begin))
		return nil
	}

	panic("unreachable")
}
