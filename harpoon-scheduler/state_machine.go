// State machine establishes a resilient connection to a remote agent, and
// faithfully represents its state in the scheduler.
package main

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

var (
	errAgentConnectionInterrupted = errors.New("agent connection interrupted")
)

// The type map[string]agent.ContainerInstance represents a set of container
// instances from a single agent. This is the type used to communicate with
// the remote agent.
//
// The type map[string]map[string]agent.ContainerInstance represents container
// instances from one or more agents. This is the type used to communicate
// with our clients. We place our current set of container instances under the
// key of our endpoint, so that our clients may multiplex updates from
// multiple state machines into a single channel, and not have to track
// individual contributors out-of-band.

type stateMachine interface {
	endpoint() string
	connected() bool
	actualBroadcaster
	taskScheduler
	quit()
}

type realStateMachine struct {
	myEndpoint string

	subc          chan chan<- map[string]map[string]agent.ContainerInstance
	unsubc        chan chan<- map[string]map[string]agent.ContainerInstance
	snapshotc     chan map[string]map[string]agent.ContainerInstance
	schedc        chan schedTaskReq
	unschedc      chan unschedTaskReq
	connectedc    chan bool
	initializec   chan map[string]agent.ContainerInstance
	updatec       chan map[string]agent.ContainerInstance
	reconnectc    chan struct{} // signal from requestLoop to connectionLoop to reconnect
	interruptionc chan struct{}
	quitc         chan struct{}

	sync.WaitGroup
}

var _ stateMachine = &realStateMachine{}

func newRealStateMachine(endpoint string, reconnect, abandon time.Duration) *realStateMachine {
	m := &realStateMachine{
		myEndpoint: endpoint,

		subc:          make(chan chan<- map[string]map[string]agent.ContainerInstance),
		unsubc:        make(chan chan<- map[string]map[string]agent.ContainerInstance),
		snapshotc:     make(chan map[string]map[string]agent.ContainerInstance),
		schedc:        make(chan schedTaskReq),
		unschedc:      make(chan unschedTaskReq),
		connectedc:    make(chan bool),
		initializec:   make(chan map[string]agent.ContainerInstance),
		updatec:       make(chan map[string]agent.ContainerInstance),
		reconnectc:    make(chan struct{}),
		interruptionc: make(chan struct{}),
		quitc:         make(chan struct{}),
	}

	m.Add(2)
	go m.connectionLoop(reconnect)
	go m.requestLoop(abandon)

	return m
}

func (m *realStateMachine) endpoint() string {
	return m.myEndpoint
}

func (m *realStateMachine) connected() bool {
	return <-m.connectedc
}

func (m *realStateMachine) subscribe(c chan<- map[string]map[string]agent.ContainerInstance) {
	m.subc <- c
}

func (m *realStateMachine) unsubscribe(c chan<- map[string]map[string]agent.ContainerInstance) {
	m.unsubc <- c
}

func (m *realStateMachine) snapshot() map[string]map[string]agent.ContainerInstance {
	return <-m.snapshotc
}

func (m *realStateMachine) schedule(_ string, id string, cfg agent.ContainerConfig) error {
	req := schedTaskReq{"", id, cfg, make(chan error)}
	m.schedc <- req
	return <-req.err
}

func (m *realStateMachine) unschedule(_, id string) error {
	req := unschedTaskReq{"", id, make(chan error)}
	m.unschedc <- req
	return <-req.err
}

func (m *realStateMachine) quit() {
	close(m.quitc)
	m.Wait()
}

func (m *realStateMachine) requestLoop(abandon time.Duration) {
	defer m.Done()

	var (
		subs     = map[chan<- map[string]map[string]agent.ContainerInstance]struct{}{}
		current  = map[string]agent.ContainerInstance{}
		tangos   = map[string]time.Time{} // container ID to be destroyed: deadline
		tangoc   = tick(1 * time.Second)
		abandonc <-chan time.Time // initially nil
	)

	client, err := agent.NewClient(m.myEndpoint)
	if err != nil {
		panic(err)
	}

	cp := func() map[string]map[string]agent.ContainerInstance {
		out := make(map[string]agent.ContainerInstance, len(current))

		for k, v := range current {
			out[k] = v
		}

		return map[string]map[string]agent.ContainerInstance{m.myEndpoint: out}
	}

	broadcast := func() {
		m := cp()
		for c := range subs {
			c <- m
		}
	}

	update := func(delta map[string]agent.ContainerInstance) {
		for id, instance := range delta {
			switch instance.ContainerStatus {
			case agent.ContainerStatusDeleted:
				delete(current, id)
			default:
				current[id] = instance
			}
		}
	}

	sched := func(id string, cfg agent.ContainerConfig) error {
		if abandonc != nil {
			return errAgentConnectionInterrupted
		}

		switch err := client.Put(id, cfg); err {
		case nil:
			return nil

		case agent.ErrContainerAlreadyExists:
			return client.Start(id)

		default:
			return err
		}
	}

	unsched := func(id string) error {
		if abandonc != nil {
			return errAgentConnectionInterrupted
		}

		instance, ok := current[id]
		if !ok {
			log.Printf("state machine: %s: unschedule request for %q, but it's not scheduled", m.myEndpoint, id)
			return nil
		}

		// Unscheduling is a multi-step process. Stop, wait for status
		// finished, then delete. Because all updates to remote container
		// state go through this very request loop, and we need to inspect
		// that status to proceed, we have to yield immediately. So, we
		// register a tango, i.e. a target to destroy, and wait.

		if _, ok := tangos[id]; ok {
			return fmt.Errorf("%q already being unscheduled on %s, have patience", id, m.myEndpoint)
		}

		tangos[id] = now().Add(2 * instance.ContainerConfig.Grace.Shutdown.Duration)

		return nil
	}

	dance := func() {
		for id, deadline := range tangos {
			instance, ok := current[id]
			if !ok {
				log.Printf("state machine: %s: tango %s neutralized", m.myEndpoint, id)
				delete(tangos, id)
				continue
			}

			if now().After(deadline) {
				log.Printf("state machine: %s: tango %s hit deadline, got to %s, giving up", m.myEndpoint, id, instance.ContainerStatus)
				delete(tangos, id)
				continue
			}

			// Out-of-sync conditions arise in testing from two conditions:
			//
			// - The (overly) resilient eventsource package hides connection
			//   interruptions between agent and scheduler. (Now fixed.)
			//
			// - The agent loses & kills containers on restart. (Known issue).
			//
			// Once these are fixed, it probably makes sense to simplify this
			// code, by removing out-of-sync detection and the reconnectc
			// altogether.

			switch instance.ContainerStatus {
			case agent.ContainerStatusRunning, agent.ContainerStatusFailed:
				log.Printf("state machine: %s: tango %s %s, issuing Stop", m.myEndpoint, id, instance.ContainerStatus)
				switch err := client.Stop(id); err {
				case nil:
					continue // OK, wait for next update

				case agent.ErrContainerAlreadyStopped:
					log.Printf("state machine: %s: tango %s Stop: %s -- out-of-sync", m.myEndpoint, id, err)
					m.reconnectc <- struct{}{} // re-sync on the next initialize
					continue

				case agent.ErrContainerNotExist:
					log.Printf("state machine: %s: tango %s Stop: %s -- out-of-sync", m.myEndpoint, id, err)
					delete(tangos, id)         // apparently the container is gone, sooo...
					m.reconnectc <- struct{}{} // re-sync on the next initialize
					continue

				default:
					log.Printf("state machine: %s: tango %s Stop: %s", m.myEndpoint, id, err)
					continue
				}

			case agent.ContainerStatusCreated, agent.ContainerStatusFinished:
				log.Printf("state machine: %s: tango %s %s, issuing Delete", m.myEndpoint, id, instance.ContainerStatus)
				switch err := client.Delete(id); err {
				case nil:
					continue // OK, wait for next update

				case agent.ErrContainerNotExist:
					log.Printf("state machine: %s: tango %s Delete: %s -- out-of-sync", m.myEndpoint, id, err)
					delete(tangos, id)         //  apparently the container is gone, sooo...
					m.reconnectc <- struct{}{} // re-sync on the next initialize
					continue

				default:
					log.Printf("state machine: %s: tango %s Delete: %s", m.myEndpoint, id, err)
					continue
				}

			default:
				log.Printf("state machine: %s: tango %s %s, nop", m.myEndpoint, id, instance.ContainerStatus)
				continue
			}
		}
	}

	for {
		select {
		case c := <-m.subc:
			subs[c] = struct{}{}
			go func(m map[string]map[string]agent.ContainerInstance) { c <- m }(cp())

		case c := <-m.unsubc:
			delete(subs, c)

		case m.snapshotc <- cp():

		case req := <-m.schedc:
			req.err <- sched(req.id, req.ContainerConfig)

		case req := <-m.unschedc:
			req.err <- unsched(req.id)

		case m.connectedc <- abandonc == nil:

		case current = <-m.initializec:
			abandonc = nil
			broadcast()

		case state := <-m.updatec:
			update(state)
			broadcast()

		case <-m.interruptionc:
			if abandonc == nil {
				abandonc = after(abandon)
			}

		case <-abandonc:
			current = map[string]agent.ContainerInstance{}
			broadcast()

		case <-tangoc:
			dance()

		case <-m.quitc:
			return
		}
	}
}

func (m *realStateMachine) connectionLoop(reconnect time.Duration) {
	defer m.Done()

	for {
		a, err := agent.NewClient(m.myEndpoint)
		if err != nil {
			log.Printf("state machine: %s: %s", m.myEndpoint, err)

			select {
			case <-m.quitc:
				return
			case <-after(reconnect):
				continue
			}
		}

		statec, stopper, err := a.Events()
		if err != nil {
			log.Printf("state machine: %s: %s", m.myEndpoint, err)

			select {
			case <-m.quitc:
				return
			case <-after(reconnect):
				continue
			}
		}

		log.Printf("state machine: %s: connection established", m.myEndpoint)
		incAgentConnectionsEstablished(1)

		if err := m.readLoop(statec, stopper); err != nil {
			log.Printf("state machine: %s: %s", m.myEndpoint, err)

			incAgentConnectionsInterrupted(1)

			m.interruptionc <- struct{}{} // signal

			select {
			case <-m.quitc:
				return
			case <-after(reconnect):
				continue
			}
		}

		return
	}
}

func (m *realStateMachine) readLoop(statec <-chan map[string]agent.ContainerInstance, stopper agent.Stopper) error {
	defer stopper.Stop()

	// When this function exits, we've lost our connection to the agent. When
	// that happens, we start an "abandon" timer, which (when fired) flushes
	// the `current` in the request loop, and thereby declares all the
	// instances lost. (That abandon timer in the request loop is reset when
	// we send the first successful state update over initializec.)

	first := true

	for {
		select {
		case <-m.quitc:
			return nil

		case <-m.reconnectc:
			return fmt.Errorf("request loop requested a reconnect")

		case state, ok := <-statec:
			if !ok {
				// When we detect a connection error, we'll trigger the lost
				// timer, but otherwise remain optimistic. We make no
				// immediate change to our set of instances, and enter our
				// reconnect loop, trying to get back in a good state.
				return errAgentConnectionInterrupted
			}

			incContainerEventsReceived(1)

			if first {
				m.initializec <- state // clears previous abandon timer
				first = false
				continue
			}

			m.updatec <- state
		}
	}
}
