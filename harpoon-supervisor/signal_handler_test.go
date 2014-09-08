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

func (*testSupervisor) Run(metricsTick <-chan time.Time, restartTimer func() <-chan time.Time) {}

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
	<-done
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
	sigc <- os.Interrupt

	// handler should call Subscribe
	subscribe := <-s.subscribec

	if stop := <-s.stopc; stop != syscall.SIGTERM {
		t.Fatalf("expected stop with SIGTERM, got %s", stop)
	}

	// supervisor reports down state
	subscribe <- (agent.ContainerProcessState{Up: false, Restarting: false})

	// handler should call Exit
	<-s.exitc

	// supervisor exits
	close(s.exited)

	// handler should complete
	<-done
}

func TestSignalHandlerSIGKILL(t *testing.T) {
	var (
		done = make(chan struct{})
		sigc = make(chan os.Signal, 1)
		s    = newTestSupervisor()
		h    = newSignalHandler(sigc, s)
	)

	go func() { h.Run(); done <- struct{}{} }()

	// user sends signal
	sigc <- os.Interrupt

	// handler should call Subscribe
	subscribe := <-s.subscribec

	// handler should call Stop(syscall.SIGTERM)
	if stop := <-s.stopc; stop != syscall.SIGTERM {
		t.Fatalf("expected stop with SIGTERM, got %s", stop)
	}

	// ignore SIGTERM request
	subscribe <- (agent.ContainerProcessState{Up: true, Restarting: true})

	// user sends another signal
	sigc <- os.Interrupt

	// handler should call Stop(syscall.SIGKILL)
	if stop := <-s.stopc; stop != syscall.SIGKILL {
		t.Fatalf("expected stop with SIGKILL, got %s", stop)
	}

	// subscribe supervisor is down
	subscribe <- (agent.ContainerProcessState{Up: false, Restarting: false})

	// handler should call Exit()
	<-s.exitc

	// exit supervisor
	close(s.exited)

	// wait for handler to finish
	<-done
}
