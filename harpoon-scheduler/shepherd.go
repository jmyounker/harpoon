// Shepherd manages state machines for all the agents in the scheduling
// domain, and wrangles all of their updates into a single, comprehensive
// view. Shepherd is also the unified interface for scheduling tasks.
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

var (
	defaultStateMachineReconnect = 1 * time.Second
	defaultStateMachineAbandon   = 5 * time.Minute
)

type actualBroadcaster interface {
	subscribe(c chan<- map[string]map[string]agent.ContainerInstance)
	unsubscribe(c chan<- map[string]map[string]agent.ContainerInstance)
	snapshot() map[string]map[string]agent.ContainerInstance
}

type taskScheduler interface {
	schedule(taskSpec) error
	unschedule(string, string) error
}

type shepherd interface {
	actualBroadcaster
	taskScheduler
	size() int
	quit()
}

type realShepherd struct {
	sizec     chan int
	subc      chan chan<- map[string]map[string]agent.ContainerInstance
	unsubc    chan chan<- map[string]map[string]agent.ContainerInstance
	snapshotc chan map[string]map[string]agent.ContainerInstance
	schedc    chan schedTaskReq
	unschedc  chan unschedTaskReq
	quitc     chan chan struct{}
}

var _ shepherd = &realShepherd{}

func newRealShepherd(d agentDiscovery) *realShepherd {
	s := &realShepherd{
		sizec:     make(chan int),
		subc:      make(chan chan<- map[string]map[string]agent.ContainerInstance),
		unsubc:    make(chan chan<- map[string]map[string]agent.ContainerInstance),
		snapshotc: make(chan map[string]map[string]agent.ContainerInstance),
		schedc:    make(chan schedTaskReq),
		unschedc:  make(chan unschedTaskReq),
		quitc:     make(chan chan struct{}),
	}
	go s.loop(d)
	return s
}

func (s *realShepherd) subscribe(c chan<- map[string]map[string]agent.ContainerInstance) {
	s.subc <- c
}

func (s *realShepherd) unsubscribe(c chan<- map[string]map[string]agent.ContainerInstance) {
	s.unsubc <- c
}

func (s *realShepherd) snapshot() map[string]map[string]agent.ContainerInstance {
	return <-s.snapshotc
}

func (s *realShepherd) schedule(spec taskSpec) error {
	req := schedTaskReq{spec, make(chan error)}
	s.schedc <- req
	return <-req.err
}

func (s *realShepherd) unschedule(endpoint, id string) error {
	req := unschedTaskReq{endpoint, id, make(chan error)}
	s.unschedc <- req
	return <-req.err
}

func (s *realShepherd) size() int {
	return <-s.sizec
}

func (s *realShepherd) quit() {
	q := make(chan struct{})
	s.quitc <- q
	<-q
}

func (s *realShepherd) loop(d agentDiscovery) {
	var (
		discoveryc = make(chan []string)
		endpoints  = []string{}
		machines   = map[string]stateMachine{}
		updatec    = make(chan map[string]map[string]agent.ContainerInstance)
		current    = map[string]map[string]agent.ContainerInstance{}
		subs       = map[chan<- map[string]map[string]agent.ContainerInstance]struct{}{}
	)

	cp := func() map[string]map[string]agent.ContainerInstance {
		out := make(map[string]map[string]agent.ContainerInstance, len(current))

		for endpoint, instances := range current {
			out[endpoint] = make(map[string]agent.ContainerInstance, len(instances))

			for id, instance := range instances {
				out[endpoint][id] = instance
			}
		}

		return out
	}

	d.subscribe(discoveryc)
	defer d.unsubscribe(discoveryc)

	select {
	case endpoints = <-discoveryc:
	case <-time.After(time.Millisecond):
		panic("misbehaving agent discovery")
	}

	machines = diff(machines, endpoints, updatec)
	current = initialize(updatec, endpoints)

	for {
		select {
		case endpoints = <-discoveryc:
			machines = diff(machines, endpoints, updatec)
			// The updatec will receive the first update from any new state
			// machines naturally.

		case update := <-updatec:
			for endpoint, instances := range update {
				current[endpoint] = instances // deleted instances are managed in the state machine
			}

			m := cp()

			for c := range subs {
				c <- m
			}

		case c := <-s.subc:
			subs[c] = struct{}{}
			go func(m map[string]map[string]agent.ContainerInstance) { c <- m }(cp())

		case c := <-s.unsubc:
			delete(subs, c)

		case s.snapshotc <- cp():

		case req := <-s.schedc:
			stateMachine, ok := machines[req.Endpoint]
			if !ok {
				req.err <- fmt.Errorf("endpoint %s not available", req.Endpoint)
				continue
			}
			req.err <- stateMachine.schedule(req.taskSpec)

		case req := <-s.unschedc:
			stateMachine, ok := machines[req.endpoint]
			if !ok {
				req.err <- fmt.Errorf("endpoint %s not available", req.endpoint)
				continue
			}
			req.err <- stateMachine.unschedule("", req.id)

		case s.sizec <- len(current):

		case q := <-s.quitc:
			close(q)
			return
		}
	}
}

// diff compares the existing first-gen state machines with the new set of
// endpoints. Endpoints without a state machine are created, and the updatec
// subscribed. State machines without an endpoint are stopped and deleted, and
// the updatec unsubscribed.
func diff(firstGen map[string]stateMachine, endpoints []string, updatec chan map[string]map[string]agent.ContainerInstance) map[string]stateMachine {
	var (
		secondGen = map[string]stateMachine{}
	)

	for _, endpoint := range endpoints {
		machine, ok := firstGen[endpoint]
		if !ok {
			machine = newRealStateMachine(endpoint, defaultStateMachineReconnect, defaultStateMachineAbandon)
			machine.subscribe(updatec)
		}

		secondGen[endpoint] = machine

		delete(firstGen, endpoint)
	}

	for endpoint, machine := range firstGen {
		// Lost state machines may be simply deleted. Their containers will be
		// detected as lost by interested parties whenever those parties query
		// us for the actual state of the scheduler domain.
		log.Printf("shepherd: %v lost", endpoint)
		machine.unsubscribe(updatec)
		machine.quit()
	}

	return secondGen
}

// initialize captures map[string]agent.ContainerInstance from the updatec
// until all expected endpoints are collected. If all expected endpoints
// aren't collected within a certain time, we panic.
func initialize(updatec chan map[string]map[string]agent.ContainerInstance, expected []string) map[string]map[string]agent.ContainerInstance {
	var (
		current     = map[string]map[string]agent.ContainerInstance{}
		outstanding = map[string]struct{}{}
		timeoutc    = time.After(1 * time.Second)
	)

	for _, endpoint := range expected {
		outstanding[endpoint] = struct{}{}
	}

	for {
		select {
		case update := <-updatec:
			for endpoint, instances := range update {
				current[endpoint] = instances
				delete(outstanding, endpoint)
			}

			if len(outstanding) <= 0 {
				return current
			}

		case <-timeoutc:
			log.Printf("shepherd: %d outstanding agent(s): %v", len(outstanding), outstanding)
			panic("timeout waiting for state machines to initialize")
		}
	}
}

type schedTaskReq struct {
	taskSpec
	err chan error
}

type unschedTaskReq struct {
	endpoint string
	id       string
	err      chan error
}

type mockTaskScheduler struct {
	started []taskSpec
	stopped []endpointID
}

var _ taskScheduler = &mockTaskScheduler{}

func newMockTaskScheduler() *mockTaskScheduler {
	return &mockTaskScheduler{
		started: []taskSpec{},
		stopped: []endpointID{},
	}
}

func (t *mockTaskScheduler) schedule(spec taskSpec) error {
	t.started = append(t.started, spec)
	return nil
}

func (t *mockTaskScheduler) unschedule(endpoint, id string) error {
	t.stopped = append(t.stopped, endpointID{endpoint, id})
	return nil
}

func (t *mockTaskScheduler) current(fakeEndpoint string) map[string]map[string]agent.ContainerInstance {
	var out = map[string]map[string]agent.ContainerInstance{}

	for _, spec := range t.started {
		if _, ok := out[fakeEndpoint]; !ok {
			out[fakeEndpoint] = map[string]agent.ContainerInstance{}
		}

		out[fakeEndpoint][spec.ContainerID] = agent.ContainerInstance{}
	}

	return out
}

type endpointID struct{ endpoint, id string }
