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
	subscribe(c chan<- map[string]agentState)
	unsubscribe(c chan<- map[string]agentState)
	snapshot() map[string]agentState
}

type taskScheduler interface {
	schedule(string, string, agent.ContainerConfig) error
	unschedule(string, string) error
}

type shepherd interface {
	actualBroadcaster
	taskScheduler
	size() int
	quit()
}

type realShepherd struct {
	sizec     chan chan int
	subc      chan chan<- map[string]agentState
	unsubc    chan chan<- map[string]agentState
	snapshotc chan chan map[string]agentState
	schedc    chan schedTaskReq
	unschedc  chan unschedTaskReq
	quitc     chan chan struct{}
	workQueue *workQueue
}

var _ shepherd = &realShepherd{}

func newRealShepherd(d agentDiscovery) *realShepherd {
	s := &realShepherd{
		sizec:     make(chan chan int),
		subc:      make(chan chan<- map[string]agentState),
		unsubc:    make(chan chan<- map[string]agentState),
		snapshotc: make(chan chan map[string]agentState),
		schedc:    make(chan schedTaskReq),
		unschedc:  make(chan unschedTaskReq),
		quitc:     make(chan chan struct{}),
		workQueue: newWorkQueue(),
	}
	go s.loop(d)
	return s
}

func (s *realShepherd) subscribe(c chan<- map[string]agentState) {
	s.subc <- c
}

func (s *realShepherd) unsubscribe(c chan<- map[string]agentState) {
	s.unsubc <- c
}

func (s *realShepherd) snapshot() map[string]agentState {
	req := make(chan map[string]agentState)
	s.snapshotc <- req
	return <-req
}

func (s *realShepherd) schedule(endpoint, id string, cfg agent.ContainerConfig) error {
	req := schedTaskReq{endpoint, id, cfg, make(chan error)}
	s.schedc <- req
	return <-req.err
}

func (s *realShepherd) unschedule(endpoint, id string) error {
	req := unschedTaskReq{endpoint, id, make(chan error)}
	s.unschedc <- req
	return <-req.err
}

func (s *realShepherd) size() int {
	req := make(chan int)
	s.sizec <- req
	return <-req
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
		updatec    = make(chan map[string]agentState)
		current    = map[string]agentState{}
		subs       = map[chan<- map[string]agentState]struct{}{}
	)

	cp := func() map[string]agentState {
		out := make(map[string]agentState, len(current))

		for endpoint, state := range current {
			instances := make(map[string]agent.ContainerInstance, len(state.instances))
			for id, instance := range state.instances {
				instances[id] = instance
			}

			volumes := make(map[string]struct{}, len(state.resources.volumes))
			for path := range state.resources.volumes {
				volumes[path] = struct{}{}
			}

			out[endpoint] = agentState{
				resources: freeResources{
					memory:  state.resources.memory,
					cpus:    state.resources.cpus,
					volumes: volumes,
				},
				instances: instances,
			}
		}

		return out
	}

	d.subscribe(discoveryc)
	defer d.unsubscribe(discoveryc)

	select {
	case endpoints = <-discoveryc:
	case <-after(time.Millisecond):
		panic("misbehaving agent discovery")
	}

	machines = diff(machines, endpoints, updatec)
	current = initialize(updatec, endpoints)

	for {
		select {
		case endpoints := <-discoveryc:
			s.workQueue.push(func() { machines = diff(machines, endpoints, updatec) })
			// The updatec will receive the first update from any new state
			// machines naturally.

		case update := <-updatec:
			s.workQueue.push(
				func() {
					for endpoint, instances := range update {
						current[endpoint] = instances // deleted instances are managed in the state machine
					}

					m := cp()
					for c := range subs {
						c <- m
					}
				},
			)
		case c := <-s.subc:
			s.workQueue.push(
				func() {
					subs[c] = struct{}{}
					c <- cp()
				},
			)

		case c := <-s.unsubc:
			s.workQueue.push(func() { delete(subs, c) })

		case req := <-s.snapshotc:
			s.workQueue.push(func() { req <- cp() })

		case req := <-s.schedc:
			s.workQueue.push(
				func() {
					stateMachine, ok := machines[req.endpoint]
					if !ok {
						req.err <- fmt.Errorf("endpoint %s not available", req.endpoint)
						return
					}
					req.err <- stateMachine.schedule("", req.id, req.ContainerConfig)
				},
			)

		case req := <-s.unschedc:
			s.workQueue.push(
				func() {
					stateMachine, ok := machines[req.endpoint]
					if !ok {
						req.err <- fmt.Errorf("endpoint %s not available", req.endpoint)
						return
					}
					req.err <- stateMachine.unschedule("", req.id)
				},
			)

		case req := <-s.sizec:
			s.workQueue.push(func() { req <- len(current) })

		case q := <-s.quitc:
			for _, machine := range machines {
				machine.unsubscribe(updatec)
				machine.quit()
			}
			s.workQueue.quit()
			close(q)
			return
		}
	}
}

// diff compares the existing first-gen state machines with the new set of
// endpoints. Endpoints without a state machine are created, and the updatec
// subscribed. State machines without an endpoint are stopped and deleted, and
// the updatec unsubscribed.
func diff(firstGen map[string]stateMachine, endpoints []string, updatec chan map[string]agentState) map[string]stateMachine {
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
func initialize(updatec chan map[string]agentState, expected []string) map[string]agentState {
	var (
		current     = map[string]agentState{}
		outstanding = map[string]struct{}{}
		timeoutc    = after(1 * time.Second)
	)

	for _, endpoint := range expected {
		outstanding[endpoint] = struct{}{}
	}

	if len(outstanding) <= 0 {
		return current
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
	endpoint string
	id       string
	agent.ContainerConfig
	err chan error
}

type unschedTaskReq struct {
	endpoint string
	id       string
	err      chan error
}

type mockTaskScheduler struct {
	started map[string]map[string]agent.ContainerConfig
	stopped map[string]map[string]struct{}
}

var _ taskScheduler = &mockTaskScheduler{}

func newMockTaskScheduler() *mockTaskScheduler {
	return &mockTaskScheduler{
		started: map[string]map[string]agent.ContainerConfig{},
		stopped: map[string]map[string]struct{}{},
	}
}

func (t *mockTaskScheduler) schedule(endpoint, id string, cfg agent.ContainerConfig) error {
	if _, ok := t.started[endpoint]; !ok {
		t.started[endpoint] = map[string]agent.ContainerConfig{}
	}

	t.started[endpoint][id] = cfg
	return nil
}

func (t *mockTaskScheduler) unschedule(endpoint, id string) error {
	if _, ok := t.stopped[endpoint]; !ok {
		t.stopped[endpoint] = map[string]struct{}{}
	}

	t.stopped[endpoint][id] = struct{}{}

	return nil
}
