package main

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type testContainer struct {
	startc  chan error
	signalc chan os.Signal
	waitc   chan agent.ContainerExitStatus
}

func newTestContainer() *testContainer {
	return &testContainer{
		startc:  make(chan error),
		signalc: make(chan os.Signal, 1),
		waitc:   make(chan agent.ContainerExitStatus),
	}
}

func (c *testContainer) Start() error {
	return <-c.startc
}

func (c *testContainer) Wait() agent.ContainerExitStatus {
	return <-c.waitc
}

func (c *testContainer) Metrics() agent.ContainerMetrics {
	return agent.ContainerMetrics{}
}

func (c *testContainer) Signal(sig os.Signal) {
	c.signalc <- sig
}

func TestSupervisor(t *testing.T) {
	var (
		container  = newTestContainer()
		supervisor = newSupervisor(container)
		statec     = make(chan agent.ContainerProcessState)

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

	select {
	case container.startc <- nil:
	case <-time.After(time.Millisecond):
		panic("supervisor did not attempt to start container")
	}

	supervisor.Subscribe(statec)
	defer supervisor.Unsubscribe(statec)

	select {
	case <-statec:
	case <-time.After(time.Millisecond):
		panic("supervisor did not send a state update")
	}

	// container process OOMs
	select {
	case container.waitc <- agent.ContainerExitStatus{OOMed: true}:
	case <-time.After(time.Millisecond):
		panic("supervisor did not consume exit status")
	}

	var state agent.ContainerProcessState

	select {
	case state = <-statec:
	case <-time.After(time.Millisecond):
		panic("supervisor did not sent state update after OOM")
	}

	if state.ContainerExitStatus.OOMed != true {
		t.Fatal("status reported did not include OOM")
	}

	if state.OOMs != 1 {
		t.Fatal("expected 1 oom")
	}

	select {
	case restartTimer <- time.Now():
	case <-time.After(time.Millisecond):
		panic("supervisor did not consume restart message")
	}

	select {
	case container.startc <- nil:
	case <-time.After(time.Millisecond):
		panic("supervisor did not attempt to start container")
	}

	select {
	case state = <-statec:
	case <-time.After(time.Millisecond):
		panic("supervisor did not sent state update after restart")
	}

	if state.Up != true {
		t.Fatal("container was not restarted")
	}

	select {
	case metricsTick <- time.Now():
	case <-time.After(time.Millisecond):
		panic("supervisor did not consume metrics tick")
	}

	select {
	case <-statec:
	case <-time.After(time.Millisecond):
		panic("supervisor did not send state update after metrics tick")
	}

	select {
	case container.waitc <- agent.ContainerExitStatus{Exited: true, ExitStatus: 0}:
	case <-time.After(time.Millisecond):
		panic("supervisor did not consume container exit status")
	}

	select {
	case state = <-statec:
	case <-time.After(time.Millisecond):
		panic("supervisor did not sent state update after exit")
	}

	if state.Up || state.Restarting {
		t.Fatal("expected container exiting 0 not to be restarted")
	}

	if err := supervisor.Exit(); err != nil {
		t.Fatalf("expected supervisor to exit, got %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Millisecond):
		panic("supervisor did not terminate after exit")
	}
}

func TestSupervisorStop(t *testing.T) {
	var (
		container  = newTestContainer()
		supervisor = newSupervisor(container)
		statec     = make(chan agent.ContainerProcessState)

		done = make(chan struct{}, 1)
	)

	go func() { supervisor.Run(nil, nil); done <- struct{}{} }()

	select {
	case container.startc <- nil:
	case <-time.After(time.Millisecond):
		panic("supervisor did not attempt to start container")
	}

	supervisor.Subscribe(statec)
	defer supervisor.Unsubscribe(statec)

	select {
	case <-statec:
	case <-time.After(time.Millisecond):
		panic("supervisor did not send a state update")
	}

	if err := supervisor.Exit(); err == nil {
		t.Fatal("expected supervisor to reject call to exit while running")
	}

	supervisor.Stop(syscall.SIGTERM)

	select {
	case sig := <-container.signalc:
		if sig != syscall.SIGTERM {
			t.Fatal("expected SIGTERM, got ", sig)
		}
	case <-time.After(time.Millisecond):
		panic("supervisor did not send SIGTERM signal to container")
	}

	select {
	case container.waitc <- agent.ContainerExitStatus{}:
	case <-time.After(time.Millisecond):
		panic("unable to send exit status")
	}

	select {
	case <-statec:
	case <-time.After(time.Millisecond):
		panic("supervisor did not send a state update")
	}

	if err := supervisor.Exit(); err != nil {
		t.Fatalf("expected supervisor to exit, got %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Millisecond):
		panic("supervisor did not terminate after exit")
	}
}
