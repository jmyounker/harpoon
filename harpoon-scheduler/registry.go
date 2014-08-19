package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
)

type desiredBroadcaster interface {
	subscribe(chan<- map[string]map[string]taskSpec)
	unsubscribe(chan<- map[string]map[string]taskSpec)
	snapshot() map[string]map[string]taskSpec
}

type registry interface {
	desiredBroadcaster
	taskScheduler
}

type realRegistry struct {
	subc      chan chan<- map[string]map[string]taskSpec
	unsubc    chan chan<- map[string]map[string]taskSpec
	schedc    chan schedTaskReq
	unschedc  chan unschedTaskReq
	snapshotc chan map[string]map[string]taskSpec
	quitc     chan chan struct{}
}

var _ registry = &realRegistry{}

func newRealRegistry(filename string) *realRegistry {
	scheduled, err := load(filename)
	if err != nil {
		panic(err)
	}

	r := &realRegistry{
		subc:      make(chan chan<- map[string]map[string]taskSpec),
		unsubc:    make(chan chan<- map[string]map[string]taskSpec),
		schedc:    make(chan schedTaskReq),
		unschedc:  make(chan unschedTaskReq),
		snapshotc: make(chan map[string]map[string]taskSpec),
		quitc:     make(chan chan struct{}),
	}

	go r.loop(filename, scheduled)

	return r
}

func (r *realRegistry) subscribe(c chan<- map[string]map[string]taskSpec) {
	r.subc <- c
}

func (r *realRegistry) unsubscribe(c chan<- map[string]map[string]taskSpec) {
	r.unsubc <- c
}

func (r *realRegistry) schedule(spec taskSpec) error {
	req := schedTaskReq{spec, make(chan error)}
	r.schedc <- req
	return <-req.err
}

func (r *realRegistry) unschedule(endpoint, id string) error {
	req := unschedTaskReq{endpoint, id, make(chan error)}
	r.unschedc <- req
	return <-req.err
}

func (r *realRegistry) snapshot() map[string]map[string]taskSpec {
	return <-r.snapshotc
}

func (r *realRegistry) quit() {
	q := make(chan struct{})
	r.quitc <- q
	<-q
}

func (r *realRegistry) loop(filename string, scheduled map[string]map[string]taskSpec) {
	schedule := func(spec taskSpec) error {
		if _, ok := scheduled[spec.Endpoint]; !ok {
			scheduled[spec.Endpoint] = map[string]taskSpec{}
		}

		existing, ok := scheduled[spec.Endpoint][spec.ContainerID]
		if ok {
			if reflect.DeepEqual(spec, existing) {
				return fmt.Errorf("%s already scheduled on %s", spec.ContainerID, spec.Endpoint)
			}
			return fmt.Errorf("%s scheduled on %s with a different config (bad state!)", spec.ContainerID, spec.Endpoint)
		}

		scheduled[spec.Endpoint][spec.ContainerID] = spec

		return nil
	}

	unschedule := func(endpoint, id string) error {
		if _, ok := scheduled[endpoint]; !ok {
			return fmt.Errorf("%s already removed from scheduler registry (%s has nothing scheduled)", id, endpoint)
		}

		if _, ok := scheduled[endpoint][id]; !ok {
			return fmt.Errorf("%s already removed from scheduler registry (it isn't scheduled on %s)", id, endpoint)
		}

		delete(scheduled[endpoint], id)

		if len(scheduled[endpoint]) <= 0 {
			delete(scheduled, endpoint)
		}

		return nil
	}

	persist := func() {
		if err := save(filename, scheduled); err != nil {
			panic(err) // TODO(pb): remove this before going live :)
		}
	}

	subscriptions := map[chan<- map[string]map[string]taskSpec]struct{}{}

	broadcast := func() {
		for c := range subscriptions {
			c <- scheduled
		}
	}

	for {
		select {
		case c := <-r.subc:
			subscriptions[c] = struct{}{}
			go func() { c <- scheduled }()

		case c := <-r.unsubc:
			delete(subscriptions, c)

		case req := <-r.schedc:
			incTaskScheduleRequests(1)

			err := schedule(req.taskSpec)
			if err == nil {
				persist()
				broadcast()
			}

			req.err <- err

		case req := <-r.unschedc:
			incTaskUnscheduleRequests(1)

			err := unschedule(req.endpoint, req.id)
			if err == nil {
				persist()
				broadcast()
			}

			req.err <- err

		case r.snapshotc <- scheduled:

		case q := <-r.quitc:
			close(q)
			return
		}
	}
}

func save(filename string, scheduled map[string]map[string]taskSpec) error {
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

func load(filename string) (map[string]map[string]taskSpec, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return map[string]map[string]taskSpec{}, nil // no file is OK
	} else if err != nil {
		return map[string]map[string]taskSpec{}, err
	}

	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return map[string]map[string]taskSpec{}, err
	}

	var scheduled map[string]map[string]taskSpec
	if err := json.Unmarshal(buf, &scheduled); err != nil {
		return map[string]map[string]taskSpec{}, err
	}

	return scheduled, nil
}
