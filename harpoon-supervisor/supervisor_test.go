package main

import (
	"os"
	"syscall"
	"testing"
	"time"
)

type testContainer struct {
	startc  chan error
	signalc chan os.Signal
	waitc   chan ContainerExitStatus
}

func newTestContainer() *testContainer {
	return &testContainer{
		startc:  make(chan error),
		signalc: make(chan os.Signal, 1),
		waitc:   make(chan ContainerExitStatus),
	}
}

func (c *testContainer) Start() error {
	return <-c.startc
}

func (c *testContainer) Wait() ContainerExitStatus {
	return <-c.waitc
}

func (c *testContainer) Metrics() ContainerMetrics {
	return ContainerMetrics{}
}

func (c *testContainer) Signal(sig os.Signal) {
	c.signalc <- sig
}

func TestSupervisor(t *testing.T) {
	var (
		container  = newTestContainer()
		supervisor = newSupervisor(container)
		statec     = make(chan ContainerProcessState)

		metricsTick  = make(chan time.Time)
		restartTimer = make(chan time.Time)

		done = make(chan struct{}, 1)
	)

	go func() {
		supervisor.Run(metricsTick, func() <-chan time.Time {
			return restartTimer
		})
		done <- struct{}{}
	}()

	// signal that container started successfully
	container.startc <- nil

	supervisor.Notify(statec)
	defer supervisor.Unnotify(statec)

	// check notification of initial state
	<-statec

	// container process OOMs
	container.waitc <- ContainerExitStatus{OOMed: true}

	{
		state := <-statec
		if state.ContainerExitStatus.OOMed != true {
			t.Fatal("status reported did not include OOM")
		}

		if state.OOMs != 1 {
			t.Fatal("expected 1 oom")
		}
	}

	// signal restart
	restartTimer <- time.Now()

	// container started without error
	container.startc <- nil

	// wait for notification that it was restarted
	{
		state := <-statec
		if state.Up != true {
			t.Fatal("container was not restarted")
		}
	}

	// tick
	metricsTick <- time.Now()

	// wait for periodic notification
	<-statec

	// container process exits
	container.waitc <- ContainerExitStatus{Exited: true, ExitStatus: 0}

	// wait for notification
	{
		state := <-statec
		if state.Up || state.Restarting {
			t.Fatal("expected container exiting 0 not to be restarted")
		}
	}

	if err := supervisor.Exit(); err != nil {
		t.Fatalf("expected supervisor to exit, got %v", err)
	}

	<-done
}

func TestSupervisorStop(t *testing.T) {
	var (
		container  = newTestContainer()
		supervisor = newSupervisor(container)
		statec     = make(chan ContainerProcessState)

		done = make(chan struct{}, 1)
	)

	go func() { supervisor.Run(nil, nil); done <- struct{}{} }()

	// signal that container started successfully
	container.startc <- nil

	supervisor.Notify(statec)
	defer supervisor.Unnotify(statec)

	// check notification of initial state
	<-statec

	if err := supervisor.Exit(); err == nil {
		t.Fatal("expected supervisor to reject call to exit while running")
	}

	supervisor.Stop(syscall.SIGTERM)

	if sig := <-container.signalc; sig != syscall.SIGTERM {
		t.Fatalf("expected SIGTERM signal, got %s", sig)
	}

	// container process exits
	container.waitc <- ContainerExitStatus{}

	// wait for notification
	<-statec

	if err := supervisor.Exit(); err != nil {
		t.Fatalf("expected supervisor to exit, got %v", err)
	}

	<-done
}
