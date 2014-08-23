package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

type desiredBroadcaster interface {
	subscribe(chan<- map[string]configstore.JobConfig)
	unsubscribe(chan<- map[string]configstore.JobConfig)
	snapshot() map[string]configstore.JobConfig
}

type jobScheduler interface {
	schedule(configstore.JobConfig) error
	unschedule(configstore.JobConfig) error
}

type registry interface {
	desiredBroadcaster
	jobScheduler
}

type realRegistry struct {
	subc      chan chan<- map[string]configstore.JobConfig
	unsubc    chan chan<- map[string]configstore.JobConfig
	schedc    chan schedJobReq
	unschedc  chan schedJobReq
	snapshotc chan map[string]configstore.JobConfig
	quitc     chan chan struct{}
}

var _ registry = &realRegistry{}

func newRealRegistry(filename string) *realRegistry {
	scheduled, err := load(filename)
	if err != nil {
		panic(err)
	}

	r := &realRegistry{
		subc:      make(chan chan<- map[string]configstore.JobConfig),
		unsubc:    make(chan chan<- map[string]configstore.JobConfig),
		schedc:    make(chan schedJobReq),
		unschedc:  make(chan schedJobReq),
		snapshotc: make(chan map[string]configstore.JobConfig),
		quitc:     make(chan chan struct{}),
	}

	go r.loop(filename, scheduled)

	return r
}

func (r *realRegistry) subscribe(c chan<- map[string]configstore.JobConfig) {
	r.subc <- c
}

func (r *realRegistry) unsubscribe(c chan<- map[string]configstore.JobConfig) {
	r.unsubc <- c
}

func (r *realRegistry) schedule(cfg configstore.JobConfig) error {
	req := schedJobReq{cfg, make(chan error)}
	r.schedc <- req
	return <-req.err
}

func (r *realRegistry) unschedule(cfg configstore.JobConfig) error {
	req := schedJobReq{cfg, make(chan error)}
	r.unschedc <- req
	return <-req.err
}

func (r *realRegistry) snapshot() map[string]configstore.JobConfig {
	return <-r.snapshotc
}

func (r *realRegistry) quit() {
	q := make(chan struct{})
	r.quitc <- q
	<-q
}

func (r *realRegistry) loop(filename string, scheduled map[string]configstore.JobConfig) {
	cp := func() map[string]configstore.JobConfig {
		out := make(map[string]configstore.JobConfig, len(scheduled))

		for id, spec := range scheduled {
			out[id] = spec
		}

		return out
	}

	schedule := func(cfg configstore.JobConfig) error {
		hash := cfg.Hash()

		if _, ok := scheduled[hash]; ok {
			return fmt.Errorf("%s already scheduled", hash)
		}

		scheduled[hash] = cfg

		return nil
	}

	unschedule := func(cfg configstore.JobConfig) error {
		hash := cfg.Hash()

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

	var (
		subscriptions = map[chan<- map[string]configstore.JobConfig]struct{}{}
	)

	broadcast := func() {
		m := cp()
		for c := range subscriptions {
			c <- m
		}
	}

	for {
		select {
		case c := <-r.subc:
			subscriptions[c] = struct{}{}
			go func(m map[string]configstore.JobConfig) { c <- m }(cp())

		case c := <-r.unsubc:
			delete(subscriptions, c)

		case req := <-r.schedc:
			incTaskScheduleRequests(1)

			err := schedule(req.JobConfig)
			if err == nil {
				persist()
				broadcast()
			}

			req.err <- err

		case req := <-r.unschedc:
			incTaskUnscheduleRequests(1)

			err := unschedule(req.JobConfig)
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

func save(filename string, scheduled map[string]configstore.JobConfig) error {
	if filename == "" {
		return nil // no file (and no persistence) is OK
	}

	// Ensure that the temp file is in the same filesystem as the registry
	// save file so that os.Rename() never crosses a filesystem boundary.
	f, err := ioutil.TempFile(filepath.Dir(filename), "harpoon-scheduler-registry_")
	if err != nil {
		return err
	}

	if err := json.NewEncoder(f).Encode(scheduled); err != nil {
		f.Close()
		return err
	}

	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}

	f.Close()

	return os.Rename(f.Name(), filename) // atomic
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

type schedJobReq struct {
	configstore.JobConfig
	err chan error
}
