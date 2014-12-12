package main

import (
	//"fmt"
	"syscall"
	"testing"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestSupervisorTransitionsToUp(t *testing.T) {
	fx := newSupervisorFixture(t)
	fx.startContainer()
	fx.subscribe()
	defer fx.cleanup()

	// Container will have sent at least one 'Up' status which we must consume before we
	// can get to the exit message.
	fx.expectStatusUpdate(agent.SupervisorStatusUp)
}

func TestSupervisorTermOOM(t *testing.T) {
	fx := newSupervisorFixture(t)
	fx.startContainer()
	fx.subscribe()
	defer fx.cleanup()

	// Pretend the container exited with an OOM
	select {
	case fx.container.waitc <- agent.ContainerExitStatus{OOMed: true}:
	case <-time.After(time.Millisecond):
		panic("supervisor did not consume exit status")
	}

	// Container will have sent at least one 'Up' status which we must consume before we
	// can get to the exit message.
	fx.expectStatusUpdate(agent.SupervisorStatusUp)

	// Check for an exit message
	state := fx.expectStatusUpdate(agent.SupervisorStatusDown)

	// The termination cause should be an out of memory
	if state.ContainerExitStatus.OOMed != true {
		t.Fatal("status reported did not include OOM")
	}
}

func TestSupervisorStopCallSendsSignalToContainer(t *testing.T) {
	fx := newSupervisorFixture(t)
	fx.startContainer()
	fx.subscribe()
	defer fx.cleanup()
	fx.expectStatusUpdate(agent.SupervisorStatusUp)

	// Send stop signal to supervisor
	fx.supervisor.Stop(syscall.SIGTERM)

	// Make sure the supervisor's container intercepted the signal
	select {
	case sig := <-fx.container.signalc:
		if sig != syscall.SIGTERM {
			t.Fatal("expected SIGTERM, got ", sig)
		}
	case <-time.After(time.Millisecond):
		panic("supervisor did not send SIGTERM signal to container")
	}
}

func TestSupervisorRejectsExitCallWhenRunning(t *testing.T) {
	fx := newSupervisorFixture(t)
	fx.startContainer()
	fx.subscribe()
	defer fx.cleanup()
	fx.expectStatusUpdate(agent.SupervisorStatusUp)

	// Send exit.  Should ignore call.
	if err := fx.supervisor.Exit(); err == nil {
		t.Fatal("expected supervisor to reject call to exit while running")
	}
}

func TestSupervisorStop(t *testing.T) {
	fx := newSupervisorFixture(t)
	fx.startContainer()
	fx.subscribe()
	defer fx.cleanup()

	// Send exit status to container
	select {
	case fx.container.waitc <- agent.ContainerExitStatus{Exited: true, ExitStatus: 0}:
	case <-time.After(time.Millisecond):
		panic("unable to send exit status")
	}

	fx.expectStatusUpdate(agent.SupervisorStatusUp)

	// Container should transition to down
	fx.expectStatusUpdate(agent.SupervisorStatusDown)

	// Exit should be allowed from the down state
	if err := fx.supervisor.Exit(); err != nil {
		t.Fatalf("expected supervisor to exit, got %v", err)
	}

	// But before it shut down it should have sent out an Exit message
	fx.expectStatusUpdate(agent.SupervisorStatusExit)

	//	Supervisor should shut down in response to exit
	select {
	case <-fx.done:
	case <-time.After(time.Millisecond):
		panic("supervisor did not terminate after exit")
	}
}

type supervisorFixture struct {
	t           *testing.T
	container   *fakeContainer
	supervisor  Supervisor
	statec      chan agent.ContainerProcessState
	metricsTick chan time.Time
	// Indicates that the test supervisor has exited
	done chan struct{}
}

func newSupervisorFixture(t *testing.T) *supervisorFixture {
	container := newFakeContainer(agent.OnFailureRestart)
	fx := &supervisorFixture{
		t:           t,
		container:   container,
		supervisor:  newSupervisor(container),
		statec:      make(chan agent.ContainerProcessState),
		metricsTick: make(chan time.Time),
		done:        make(chan struct{}, 1),
	}

	go func() {
		fx.supervisor.Run(fx.metricsTick)
		fx.done <- struct{}{}
	}()

	return fx
}

func (fx *supervisorFixture) startContainer() {
	select {
	case fx.container.startc <- nil:
	case <-time.After(time.Millisecond):
		panic("supervisor did not attempt to start container")
	}
}

func (fx *supervisorFixture) subscribe() {
	fx.supervisor.Subscribe(fx.statec)
}

func (fx *supervisorFixture) cleanup() {
	fx.supervisor.Unsubscribe(fx.statec)
}

func (fx *supervisorFixture) expectStatusUpdate(status agent.SupervisorStatus) agent.ContainerProcessState {
	select {
	case state := <-fx.statec:
		if state.SupervisorStatus != status {
			fx.t.Fatalf("wanted status '%s' but got status '%s'", status, state.SupervisorStatus)
		}
		return state
	case <-time.After(time.Millisecond):
		panic("supervisor did not send a state update")
	}
}
