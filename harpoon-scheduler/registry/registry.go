// Package registry deals with storing and persisting the expressed desired
// state of the scheduling domain.
package registry

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
	"github.com/soundcloud/harpoon/harpoon-scheduler/metrics"
)

// Registry accepts job schedule and unschedule requests, and persists them to
// storage. It also broadcasts all updates to any subscribers who care to
// listen.
type Registry struct {
	subc      chan chan<- map[string]configstore.JobConfig
	unsubc    chan chan<- map[string]configstore.JobConfig
	schedc    chan scheduleRequest
	unschedc  chan unscheduleRequest
	snapshotc chan map[string]configstore.JobConfig
	quitc     chan chan struct{}
}

// New constructs a new Registry. It will restore state from the passed
// filename, if it exists, and persist all mutations there.
func New(filename string) *Registry {
	scheduled, err := load(filename)
	if err != nil {
		panic(err)
	}

	r := &Registry{
		subc:      make(chan chan<- map[string]configstore.JobConfig),
		unsubc:    make(chan chan<- map[string]configstore.JobConfig),
		schedc:    make(chan scheduleRequest),
		unschedc:  make(chan unscheduleRequest),
		snapshotc: make(chan map[string]configstore.JobConfig),
		quitc:     make(chan chan struct{}),
	}

	go r.loop(filename, scheduled)

	return r
}

// Subscribe implements xf.DesireBroadcaster.
func (r *Registry) Subscribe(c chan<- map[string]configstore.JobConfig) {
	r.subc <- c
}

// Unsubscribe implements xf.DesireBroadcaster.
func (r *Registry) Unsubscribe(c chan<- map[string]configstore.JobConfig) {
	r.unsubc <- c
}

// Schedule implements api.JobScheduler.
func (r *Registry) Schedule(c configstore.JobConfig) error {
	req := scheduleRequest{
		JobConfig: c,
		err:       make(chan error),
	}
	r.schedc <- req
	return <-req.err
}

// Unschedule implements api.JobScheduler.
func (r *Registry) Unschedule(jobConfigHash string) error {
	req := unscheduleRequest{
		hash: jobConfigHash,
		err:  make(chan error),
	}
	r.unschedc <- req
	return <-req.err
}

// Snapshot implements api.JobScheduler.
func (r *Registry) Snapshot() map[string]configstore.JobConfig {
	return <-r.snapshotc
}

// Quit terminates the Registry.
func (r *Registry) Quit() {
	q := make(chan struct{})
	r.quitc <- q
	<-q
}

func (r *Registry) loop(filename string, scheduled map[string]configstore.JobConfig) {
	var (
		subs = map[chan<- map[string]configstore.JobConfig]struct{}{}
	)

	cp := func() map[string]configstore.JobConfig {
		out := make(map[string]configstore.JobConfig, len(scheduled))

		for id, spec := range scheduled {
			out[id] = spec
		}

		return out
	}

	schedule := func(config configstore.JobConfig) error {
		hash := config.Hash()

		if _, ok := scheduled[hash]; ok {
			return fmt.Errorf("%s already scheduled", hash)
		}

		scheduled[hash] = config

		return nil
	}

	unschedule := func(hash string) error {
		if _, ok := scheduled[hash]; !ok {
			return fmt.Errorf("%s not scheduled", hash)
		}

		delete(scheduled, hash)

		return nil
	}

	persist := func() {
		if err := save(filename, scheduled); err != nil {
			panic(err) // TODO(pb): remove this before going live :)
		}
	}

	broadcast := func() {
		m := cp()
		for c := range subs {
			c <- m
		}
	}

	for {
		select {
		case c := <-r.subc:
			subs[c] = struct{}{}
			go func(m map[string]configstore.JobConfig) { c <- m }(cp())

		case c := <-r.unsubc:
			delete(subs, c)

		case req := <-r.schedc:
			metrics.IncJobScheduleRequests(1)

			err := schedule(req.JobConfig)
			if err == nil {
				persist()
				broadcast()
			}

			req.err <- err

		case req := <-r.unschedc:
			metrics.IncJobUnscheduleRequests(1)

			err := unschedule(req.hash)
			if err == nil {
				persist()
				broadcast()
			}

			req.err <- err

		case r.snapshotc <- cp():

		case q := <-r.quitc:
			close(q)
			return
		}
	}
}

func save(filename string, scheduled map[string]configstore.JobConfig) (err error) {
	if filename == "" {
		return nil // no file (and no persistence) is OK
	}

	// Ensure that the temp file is in the same filesystem as the registry
	// save file so that os.Rename() never crosses a filesystem boundary.
	f, err := ioutil.TempFile(filepath.Dir(filename), "harpoon-scheduler-registry")
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			os.Remove(f.Name())
		}
	}()

	defer f.Close()

	if err := json.NewEncoder(f).Encode(scheduled); err != nil {
		return err
	}

	if err = f.Sync(); err != nil {
		return err
	}

	f.Close() // double close is OK, I think

	if err := os.Rename(f.Name(), filename); err != nil {
		return err
	}

	return nil
}

func load(filename string) (map[string]configstore.JobConfig, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return map[string]configstore.JobConfig{}, nil // no file is OK
	} else if err != nil {
		return map[string]configstore.JobConfig{}, err
	}

	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return map[string]configstore.JobConfig{}, err
	}

	var scheduled map[string]configstore.JobConfig
	if err := json.Unmarshal(buf, &scheduled); err != nil {
		return map[string]configstore.JobConfig{}, err
	}

	return scheduled, nil
}

type scheduleRequest struct {
	configstore.JobConfig
	err chan error
}

type unscheduleRequest struct {
	hash string
	err  chan error
}
