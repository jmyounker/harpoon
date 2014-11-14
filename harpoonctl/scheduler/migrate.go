package scheduler

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var migrateCommand = cli.Command{
	Name:        "migrate",
	Usage:       "migrate old.json new.json",
	Description: "Interactively migrate an existing job (old.json) to a new configuration (new.json).",
	Action:      migrateAction,
	Flags:       []cli.Flag{},
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
		irqc   = make(chan os.Signal) // ctrl-C
		abortc = make(chan struct{})  // into migrate goroutine
		errc   = make(chan error)     // out of migrate goroutine
	)

	signal.Notify(irqc, syscall.SIGINT, syscall.SIGTERM)
	go func() { errc <- migrate(oldJob, newJob, abortc) }()

	select {
	case sig := <-irqc:
		log.Errorf("received %s, terminating migration...", sig)
		close(abortc)
		log.Fatalf("migration failed: %s", <-errc)

	case err := <-errc:
		if err != nil {
			log.Fatalf("migration failed: %s", err)
		}
	}

	log.Printf("migration complete")
}

func migrate(oldJob, newJob configstore.JobConfig, abortc chan struct{}) error {
	// TODO(pb): wire in abortc everywhere

	if err := schedule(newJob); err != nil {
		return fmt.Errorf("request to schedule new job failed: %s", err)
	}

	// TODO(pb): wait for all new tasks to be running

	if err := unscheduleConfig(oldJob); err != nil {
		return fmt.Errorf("request to unschedule old job failed: %s", err)
	}

	// TODO(pb): wait for all old tasks to be terminated

	return nil
}
