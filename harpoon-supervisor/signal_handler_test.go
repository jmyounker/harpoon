package main

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type testSupervisor struct {
	subscribec   chan chan<- agent.ContainerProcessState
	unsubscribec chan chan<- agent.ContainerProcessState
	stopc        chan os.Signal
	exitc        chan struct{}
	exited       chan struct{}
}

func (*testSupervisor) Run(metricsTick <-chan time.Time) {}

func (s *testSupervisor) Subscribe(c chan<- agent.ContainerProcessState) {
	s.subscribec <- c
}

func (s *testSupervisor) Unsubscribe(c chan<- agent.ContainerProcessState) {
	s.unsubscribec <- c
}

func (s *testSupervisor) Stop(sig os.Signal) {
	s.stopc <- sig
}

func (s *testSupervisor) Exit() error {
	s.exitc <- struct{}{}
	return nil
}

func (s *testSupervisor) Exited() <-chan struct{} {
	return s.exited
}

func newTestSupervisor() *testSupervisor {
	return &testSupervisor{
		subscribec:   make(chan chan<- agent.ContainerProcessState, 1),
		unsubscribec: make(chan chan<- agent.ContainerProcessState, 1),
		stopc:        make(chan os.Signal, 1),
		exitc:        make(chan struct{}, 1),
		exited:       make(chan struct{}),
	}
}

func TestSignalHandlerSupervisorExit(t *testing.T) {
	var (
		done = make(chan struct{})
		sigc = make(chan os.Signal)
		s    = newTestSupervisor()
		h    = newSignalHandler(sigc, s)
	)

	go func() { h.Run(); done <- struct{}{} }()

	// supervisor exits
	close(s.exited)

	// handler completes
	select {
	case <-done:
	case <-time.After(time.Millisecond):
		panic("signal handler did not terminate after supervisor exit")
	}
}

func TestSignalHandlerSIGTERM(t *testing.T) {
	var (
		done = make(chan struct{})
		sigc = make(chan os.Signal, 1)
		s    = newTestSupervisor()
		h    = newSignalHandler(sigc, s)
	)

	go func() { h.Run(); done <- struct{}{} }()

	// user sends interrupt
	select {
	case sigc <- os.Interrupt:
	case <-time.After(time.Millisecond):
		panic("signal handler did not receive signal")
	}

	var subscribe chan<- agent.ContainerProcessState

	select {
	case subscribe = <-s.subscribec:
	case <-time.After(time.Millisecond):
		panic("signal handler did not subscribe to supervisor")
	}

	select {
	case stop := <-s.stopc:
		if stop != syscall.SIGTERM {
			t.Fatal("expected SIGTERM, got ", stop)
		}
	case <-time.After(time.Millisecond):
		panic("signal handler did not supervisor.Stop")
	}

	// supervisor reports down state
	select {
	case subscribe <- (agent.ContainerProcessState{Up: false, Restarting: false}):
	case <-time.After(time.Millisecond):
		panic("unable to send state update to signal handler")
	}

	select {
	case <-s.exitc:
	case <-time.After(time.Millisecond):
		panic("handler did not call exit on supervisor")
	}

	// supervisor exits
	close(s.exited)

	select {
	case <-done:
	case <-time.After(time.Millisecond):
		panic("signal handler did not terminate after supervisor exit")
	}
}

func TestSignalHandlerSIGKILL(t *testing.T) {
	var (
		done = make(chan struct{})
		sigc = make(chan os.Signal)
		s    = newTestSupervisor()
		h    = newSignalHandler(sigc, s)
	)

	go func() { h.Run(); done <- struct{}{} }()

	// user sends signal
	select {
	case sigc <- os.Interrupt:
	case <-time.After(time.Millisecond):
		panic("signal handler did not receive signal")
	}

	var subscribe chan<- agent.ContainerProcessState

	select {
	case subscribe = <-s.subscribec:
	case <-time.After(time.Millisecond):
		panic("signal handler did not subscribe to supervisor")
	}

	select {
	case stop := <-s.stopc:
		if stop != syscall.SIGTERM {
			t.Fatal("expected SIGTERM, got ", stop)
		}
	case <-time.After(time.Millisecond):
		panic("signal handler did not supervisor.Stop")
	}

	// ignore SIGTERM request
	select {
	case subscribe <- (agent.ContainerProcessState{Up: true, Restarting: true}):
	case <-time.After(time.Millisecond):
		panic("unable to send state update to signal handler")
	}

	// user sends another signal
	select {
	case sigc <- os.Interrupt:
	case <-time.After(time.Millisecond):
		panic("signal handler did not receive signal")
	}

	select {
	case stop := <-s.stopc:
		if stop != syscall.SIGKILL {
			t.Fatal("expected SIGKILL, got ", stop)
		}
	case <-time.After(time.Millisecond):
		panic("signal handler did not supervisor.Stop")
	}

	select {
	case subscribe <- (agent.ContainerProcessState{Up: false, Restarting: false}):
	case <-time.After(time.Millisecond):
		panic("unable to send state update to signal handler")
	}

	select {
	case <-s.exitc:
	case <-time.After(time.Millisecond):
		panic("handler did not call exit on supervisor")
	}

	// exit supervisor
	close(s.exited)

	select {
	case <-s.unsubscribec:
	case <-time.After(time.Millisecond):
		panic("handler did not unsubscribe after supervisor exit")
	}

	select {
	case <-done:
	case <-time.After(time.Millisecond):
		panic("signal handler did not terminate after supervisor exit")
	}
}
