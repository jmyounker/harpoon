// The remote agent state machine represents a remote agent instance in the
// scheduler domain. It opens and maintains an event stream, so it can
// represent the current state of the remote agent.
//
// Components in the scheduler domain that need information about specific
// agents (e.g. a scheduling algorithm) query remote agent state machines to
// make their decisions.
package main

import (
	"fmt"
	"log"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type stateMachine struct {
	agent.Agent
	snapshotc chan chan map[string]agent.ContainerInstance
	dirtyc    chan chan bool
	quit      chan chan struct{}
}

func newStateMachine(endpoint string) *stateMachine {
	proxy, err := newRemoteAgent(endpoint)
	if err != nil {
		panic(fmt.Sprintf("when building agent proxy: %s", err))
	}

	// From here forward, we should be resilient, retry, etc.

	updatec, stopper, err := proxy.Events()
	if err != nil {
		panic(fmt.Sprintf("when getting agent event stream: %s", err)) // TODO(pb): don't panic!
	}

	s := &stateMachine{
		Agent:     proxy,
		snapshotc: make(chan chan map[string]agent.ContainerInstance),
		dirtyc:    make(chan chan bool),
		quit:      make(chan chan struct{}),
	}

	go s.loop(proxy.URL.String(), updatec, stopper)

	return s
}

func (s *stateMachine) dirty() bool {
	c := make(chan bool)
	s.dirtyc <- c
	return <-c
}

func (s *stateMachine) proxy() agent.Agent {
	return s.Agent
}

func (s *stateMachine) snapshot() map[string]agent.ContainerInstance {
	c := make(chan map[string]agent.ContainerInstance)
	s.snapshotc <- c
	return <-c
}

func (s *stateMachine) stop() {
	q := make(chan struct{})
	s.quit <- q
	<-q
}

func (s *stateMachine) loop(
	endpoint string,
	updatec <-chan []agent.ContainerInstance,
	stopper agent.Stopper,
) {
	defer stopper.Stop()

	var (
		dirty     = true             // indicator of trust
		instances = agentInstances{} // initially empty, pending first update (clients, check dirty flag)
	)

	for {
		select {
		case update, ok := <-updatec:
			if !ok {
				log.Printf("state machine: %s: container events chan closed", endpoint)
				log.Printf("state machine: %s: TODO: re-establish connection", endpoint)
				// TODO(pb): channel-of-channels idiom for connection mgmt
				updatec = nil // TODO re-establish connection, instead of this
				dirty = true  // TODO and some way to reset that
				continue
			}

			incContainerEventsReceived(1)

			log.Printf("state machine: %s: update (%d)", endpoint, len(update))
			for _, ci := range update {
				instances.update(ci)
			}

			dirty = false

		case c := <-s.dirtyc:
			c <- dirty

		case c := <-s.snapshotc:
			log.Printf("### %s snapshot request: %+v", endpoint, instances)
			c <- instances // client should consider dirty flag

		case q := <-s.quit:
			close(q)
			return
		}
	}
}

type agentInstances map[string]agent.ContainerInstance

func newAgentInstances(initial []agent.ContainerInstance) agentInstances {
	agentInstances := agentInstances{}
	for _, ci := range initial {
		agentInstances[ci.ID] = ci
	}
	return agentInstances
}

func (ai agentInstances) update(ci agent.ContainerInstance) {
	switch ci.Status {
	case agent.ContainerStatusCreated, agent.ContainerStatusRunning:
		ai[ci.ID] = ci
	case agent.ContainerStatusFinished, agent.ContainerStatusFailed, agent.ContainerStatusDeleted:
		delete(ai, ci.ID)
	default:
		panic(fmt.Sprintf("container status %q unhandled", ci.Status))
	}
}

type stateRequest struct {
	resp chan map[string]agent.ContainerInstance
	err  chan error
}
