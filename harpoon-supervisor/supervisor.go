package main

import (
	"errors"
	"os"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

var errNotDown = errors.New("supervisor not down")

// A Supervisor manages a Container process.
type Supervisor interface {
	// Run starts the supervisor. It blocks until Exit is called.
	Run(metricsTick <-chan time.Time, restartTimer func() <-chan time.Time)

	Subscribe(chan<- agent.ContainerProcessState)
	Unsubscribe(chan<- agent.ContainerProcessState)

	// Stop sends the signal to the supervised process. If the process exits it
	// will not be restarted.
	Stop(os.Signal)

	// Exit stops the supervisor. Exit returns an error if the supervised process
	// has not been stopped.
	Exit() error

	// Exited returns a channel which will be notified when the supervisor exits.
	Exited() <-chan struct{}
}

type supervisor struct {
	container Container

	subscribers  map[chan<- agent.ContainerProcessState]struct{}
	subscribec   chan chan<- agent.ContainerProcessState
	unsubscribec chan chan<- agent.ContainerProcessState
	downc        chan os.Signal
	exitc        chan chan error
	exited       chan struct{}
}

func newSupervisor(c Container) Supervisor {
	return &supervisor{
		container:    c,
		subscribers:  map[chan<- agent.ContainerProcessState]struct{}{},
		subscribec:   make(chan chan<- agent.ContainerProcessState),
		unsubscribec: make(chan chan<- agent.ContainerProcessState),
		downc:        make(chan os.Signal),
		exitc:        make(chan chan error),
		exited:       make(chan struct{}),
	}
}

func (s *supervisor) Subscribe(c chan<- agent.ContainerProcessState) {
	select {
	case s.subscribec <- c:
	case <-s.exited:
	}
}

func (s *supervisor) Unsubscribe(c chan<- agent.ContainerProcessState) {
	select {
	case s.unsubscribec <- c:
	case <-s.exited:
	}
}

func (s *supervisor) Stop(sig os.Signal) {
	select {
	case s.downc <- sig:
	case <-s.exited:
	}
}

func (s *supervisor) Exit() error {
	c := make(chan error)

	select {
	case s.exitc <- c:
	case <-s.exited:
		return nil
	}

	select {
	case err := <-c:
		return err
	case <-s.exited:
		return nil
	}
}

func (s *supervisor) Exited() <-chan struct{} {
	return s.exited
}

func (s *supervisor) Run(metricsTick <-chan time.Time, restartTimer func() <-chan time.Time) {
	var (
		state          agent.ContainerProcessState
		containerExitc chan agent.ContainerExitStatus
		restart        <-chan time.Time
	)

	defer close(s.exited)

	if err := s.container.Start(); err != nil {
		state = agent.ContainerProcessState{Err: err.Error()}
		metricsTick = nil
	} else {
		state = agent.ContainerProcessState{Up: true, Restarting: true}

		containerExitc = make(chan agent.ContainerExitStatus, 1)
		go func() { containerExitc <- s.container.Wait() }()
	}

	for {
		select {
		case <-restart:
			incContainerRestart(1)
			if err := s.container.Start(); err != nil {
				state.Err = err.Error()
				state.Restarting = false

				continue
			}

			state.Up = true
			state.Restarts++
			state.ContainerExitStatus = agent.ContainerExitStatus{}

			containerExitc = make(chan agent.ContainerExitStatus, 1)
			go func() { containerExitc <- s.container.Wait() }()
			s.broadcast(state)

		case exitStatus := <-containerExitc:
			state.Up = false
			state.ContainerExitStatus = exitStatus

			if exitStatus.OOMed {
				incContainerOom(1)
				state.OOMs++
			}

			if exitStatus.Exited && exitStatus.ExitStatus == 0 {
				state.Up = false
				state.Restarting = false
			}

			if !state.Restarting {
				metricsTick = nil
			}

			if state.Restarting {
				restart = restartTimer()
			}

			s.broadcast(state)

		case <-metricsTick:
			state.ContainerMetrics = s.container.Metrics()
			updateMetrics(s.container.Metrics())
			s.broadcast(state)

		case sig := <-s.downc:
			state.Restarting = false

			if state.Up {
				s.container.Signal(sig)
				continue
			}

			metricsTick = nil
			restart = nil
			s.broadcast(state)

		case c := <-s.subscribec:
			s.subscribers[c] = struct{}{}
			s.notify(c, state)

		case c := <-s.unsubscribec:
			delete(s.subscribers, c)

		case c := <-s.exitc:
			if state.Up || state.Restarting {
				c <- errNotDown
				continue
			}

			c <- nil
			return
		}
	}
}

// notify sends state to c, unless unsubscribe is called for c.
func (s *supervisor) notify(c chan<- agent.ContainerProcessState, state agent.ContainerProcessState) {
	for {
		select {
		case c <- state:
			return

		case l := <-s.unsubscribec:
			delete(s.subscribers, l)

			if l == c {
				return
			}
		}
	}
}

// broadcast sends state to all subscribers.
func (s *supervisor) broadcast(state agent.ContainerProcessState) {
	for c := range s.subscribers {
		s.notify(c, state)
	}
}
