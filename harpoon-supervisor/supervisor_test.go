package main

import (
	"fmt"
	"syscall"
	"testing"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestSupervisor(t *testing.T) {
	var (
		container  = newFakeContainer(agent.OnFailureRestart)
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
	case container.waitc <- agent.ContainerExitStatus{Cause: agent.TermOOM}:
	case <-time.After(time.Millisecond):
		panic("supervisor did not consume exit status")
	}

	var state agent.ContainerProcessState

	select {
	case state = <-statec:
	case <-time.After(time.Millisecond):
		panic("supervisor did not sent state update after OOM")
	}

	if state.ContainerExitStatus.Cause != agent.TermOOM {
		t.Fatal("status reported did not include OOM")
	}

	if state.OOMs != 1 {
		t.Fatal("expected 1 oom")
	}

	state, err := waitRestart(restartTimer, container, statec, state.ExitStatus)
	if err != nil {
		t.Fatal(err)
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
	case container.waitc <- agent.ContainerExitStatus{Cause: agent.TermExit, ExitStatus: 0}:
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
		container  = newFakeContainer(agent.OnFailureRestart)
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
	case container.waitc <- agent.ContainerExitStatus{Cause: agent.TermSignal}:
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

func TestAlwaysRestartPolicy(t *testing.T) {
	for exitStatus := 0; exitStatus < 2; exitStatus++ {
		var (
			container    = newFakeContainer(agent.AlwaysRestart)
			supervisor   = newSupervisor(container)
			statec       = make(chan agent.ContainerProcessState)
			restartTimer = make(chan time.Time)
			done         = make(chan struct{}, 1)
		)

		go func() {
			supervisor.Run(nil, func() <-chan time.Time {
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

		select {
		case container.waitc <- agent.ContainerExitStatus{Cause: agent.TermExit, ExitStatus: exitStatus}:
		case <-time.After(time.Millisecond):
			panic("unable to send exit status")
		}

		select {
		case state := <-statec:
			if !state.Restarting {
				t.Fatalf("container exited with %d status should be in restarting state", exitStatus)
			}
		case <-time.After(time.Millisecond):
			panic("supervisor did not send a state update")
		}

		if _, err := waitRestart(restartTimer, container, statec, exitStatus); err != nil {
			t.Fatal(err)
		}

		if err := stopSupervisor(supervisor, container, statec); err != nil {
			t.Fatal(err)
		}

		select {
		case <-done:
		case <-time.After(time.Millisecond):
			panic("supervisor did not terminate after exit")
		}
	}
}

func TestNoRestartPolicy(t *testing.T) {
	for exitStatus := 0; exitStatus < 2; exitStatus++ {
		var (
			container    = newFakeContainer(agent.NoRestart)
			supervisor   = newSupervisor(container)
			statec       = make(chan agent.ContainerProcessState)
			restartTimer = make(chan time.Time)
			done         = make(chan struct{}, 1)
		)

		go func() {
			supervisor.Run(nil, func() <-chan time.Time {
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

		select {
		case container.waitc <- agent.ContainerExitStatus{Cause: agent.TermExit, ExitStatus: exitStatus}:
		case <-time.After(time.Millisecond):
			panic("unable to send exit status")
		}

		select {
		case state := <-statec:
			if state.Restarting {
				t.Fatalf("container exited with %d status should not be in restarting state", exitStatus)
			}
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
}

func TestOnFailureRestartPolicy(t *testing.T) {
	for exitStatus := 0; exitStatus < 2; exitStatus++ {
		var (
			container    = newFakeContainer(agent.OnFailureRestart)
			supervisor   = newSupervisor(container)
			statec       = make(chan agent.ContainerProcessState)
			restartTimer = make(chan time.Time)
			done         = make(chan struct{}, 1)
		)

		go func() {
			supervisor.Run(nil, func() <-chan time.Time {
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

		select {
		case container.waitc <- agent.ContainerExitStatus{Cause: agent.TermExit, ExitStatus: exitStatus}:
		case <-time.After(time.Millisecond):
			panic("unable to send exit status")
		}

		select {
		case state := <-statec:
			if state.Restarting && exitStatus == 0 {
				t.Fatalf("container exited with %d status should not be in restarting state", exitStatus)
			}
		case <-time.After(time.Millisecond):
			panic("supervisor did not send a state update")
		}

		if exitStatus == 0 {
			if err := supervisor.Exit(); err != nil {
				t.Fatalf("expected supervisor to exit, got %v", err)
			}
		} else {
			if _, err := waitRestart(restartTimer, container, statec, exitStatus); err != nil {
				t.Fatal(err)
			}

			if err := stopSupervisor(supervisor, container, statec); err != nil {
				t.Fatal(err)
			}
		}

		select {
		case <-done:
		case <-time.After(time.Millisecond):
			panic("supervisor did not terminate after exit")
		}
	}
}

func stopSupervisor(supervisor Supervisor, container *fakeContainer, statec chan agent.ContainerProcessState) error {
	supervisor.Stop(syscall.SIGTERM)
	select {
	case <-container.signalc:
	case <-time.After(time.Second):
		return fmt.Errorf("supervisor did not send SIGTERM signal to container")
	}

	select {
	case container.waitc <- agent.ContainerExitStatus{Cause: agent.TermSignal}:
	case <-time.After(time.Millisecond):
		return fmt.Errorf("unable to send exit status")
	}

	select {
	case <-statec:
	case <-time.After(time.Millisecond):
		return fmt.Errorf("supervisor did not send a state update")
	}

	if err := supervisor.Exit(); err != nil {
		return fmt.Errorf("expected supervisor to exit, got %v", err)
	}

	return nil
}

func waitRestart(restartTimer chan time.Time, container *fakeContainer, statec chan agent.ContainerProcessState, exitStatus int) (agent.ContainerProcessState, error) {
	var state = agent.ContainerProcessState{}

	select {
	case restartTimer <- time.Now():
	case <-time.After(time.Millisecond):
		return state, fmt.Errorf("supervisor did not consume restart message after exiting with status %d", exitStatus)
	}

	select {
	case container.startc <- nil:
	case <-time.After(time.Millisecond):
		return state, fmt.Errorf("supervisor did not attempt to start container after exiting with status %d", exitStatus)
	}

	select {
	case state = <-statec:
	case <-time.After(time.Millisecond):
		return state, fmt.Errorf("supervisor did not send a state update")
	}

	return state, nil
}
