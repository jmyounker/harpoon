package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

// stateFn represents the state of a container
// as a function that returns the next state.
// http://www.youtube.com/watch?v=HxaD_trXwRE&t=829
type stateFn func(transition) stateFn

type transition int

const (
	ApiCreate              transition = iota // Api initiated create
	ApiStart                                 // Api initiated start
	ApiStop                                  // Api initiated stop
	ApiDestroy                               // Api initiated destroy
	IntCreated                               // internally triggered
	IntCreateFailed                          // internally triggered
	IntDestroyed                             // internally triggered
	TimeoutNoUpdate                          // no update message received from supervisor in the last k seconds
	TimeoutDwell                             // no state transition received since the dwell timer started
	ConfigOnFailureRestart                   // restart only when in failed state
	ConfigAlwaysRestart                      // restart when in failed or finished state
	SupStarting                              // supervisor has moved to starting state
	SupUp                                    // supervisor says container is up
	SupSignal                                // supervisor says container terminated with a signal
	SupExitFail                              // supervisor says container terminated with exit != 0
	SupExitOK                                // supervisor says container terminated with exit == 0
	SupExitOOM                               // supervisor says container terminated with OOM
)

func (fn stateFn) String() string {
	if fn == nil {
		return "nil"
	}

	full := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
	if full == "" {
		return "nil"
	}

	toks := strings.Split(full, "/")
	last := toks[len(toks)-1]
	return strings.SplitN(last, ".", 2)[1]
}

func (fn stateFn) ContainerStatus() agent.ContainerStatus {
	switch fn.String() {
	case "initialState":
		return agent.ContainerStatusInitial
	case "creatingState":
		return agent.ContainerStatusCreating
	case "createdState":
		return agent.ContainerStatusCreated
	case "destroyedState":
		return agent.ContainerStatusDeleted
	case "startingState":
		return agent.ContainerStatusStarting
	case "runningState":
		return agent.ContainerStatusRunning
	case "stoppingState":
		return agent.ContainerStatusStopping
	case "stoppedState":
		return agent.ContainerStatusStopped
	case "failedState":
		return agent.ContainerStatusFailed
	case "finishedState":
		return agent.ContainerStatusFinished
	case "restartWaitState":
		return agent.ContainerStatusRestartWait
	default:
		panic(fmt.Sprintf("unknown container status function: %s", fn.String()))
	}
}

func initialState(t transition) stateFn {
	switch t {
	case ApiStart:
		return initialState
	case ApiStop:
		return initialState
	case ApiDestroy:
		return initialState
	case ApiCreate:
		return creatingState
	default:
		panic("unreachable")
	}
}

func creatingState(t transition) stateFn {
	switch t {
	case ApiCreate:
		return creatingState
	case ApiStart:
		return creatingState
	case ApiStop:
		return creatingState
	case ApiDestroy:
		return creatingState
	case IntCreated:
		return createdState
	case IntCreateFailed:
		return destroyedState
	case TimeoutDwell:
		return destroyedState
	default:
		panic("unreachable")
	}
}

func createdState(t transition) stateFn {
	switch t {
	case ApiCreate:
		return createdState
	case ApiStart:
		return startingState
	case ApiStop:
		return createdState
	case ApiDestroy:
		return destroyedState
	case SupStarting: // this can happen when container is recovered
		return startingState
	case SupUp: // this can happen when container is recovered
		return runningState
	default:
		panic("unreachable")
	}
}

func destroyedState(t transition) stateFn {
	switch t {
	case ApiCreate:
		return destroyedState
	case ApiStart:
		return destroyedState
	case ApiStop:
		return destroyedState
	case ApiDestroy:
		return destroyedState
	case IntDestroyed: // may not be necessary
		return nil
	default:
		panic("unreachable")
	}
}

func startingState(t transition) stateFn {
	switch t {
	case ApiCreate:
		return startingState
	case ApiStart:
		return startingState
	case ApiStop:
		return startingState
	case ApiDestroy:
		return startingState
	case SupStarting:
		return startingState
	case SupUp:
		return runningState
	case SupSignal:
		return failedState
	case SupExitFail:
		return failedState
	case SupExitOK:
		return finishedState
	case SupExitOOM:
		return failedState
	case TimeoutNoUpdate: // do we need a killing state?
		return failedState
	case TimeoutDwell: // do we need a killing state?
		return failedState
	default:
		panic("unreachable")
	}
}

func runningState(t transition) stateFn {
	switch t {
	case ApiCreate:
		return runningState
	case ApiStart:
		return runningState
	case ApiStop:
		return stoppingState
	case ApiDestroy:
		return runningState
	case SupStarting:
		return startingState
	case SupUp:
		return runningState
	case SupExitFail:
		return failedState
	case SupExitOK:
		return finishedState
	case SupSignal:
		return failedState
	case SupExitOOM:
		return failedState
	case TimeoutNoUpdate: // do we need a killing state?
		return failedState
	default:
		panic("unreachable")
	}
}

func stoppingState(t transition) stateFn {
	switch t {
	case ApiCreate:
		return stoppingState
	case ApiStart:
		return stoppingState
	case ApiStop:
		return stoppingState
	case ApiDestroy:
		return stoppingState
	case SupStarting:
		return startingState
	case SupUp:
		return runningState
	case SupExitFail:
		return stoppedState
	case SupExitOK:
		return stoppedState
	case SupSignal:
		return stoppedState
	case SupExitOOM:
		return stoppedState
	case TimeoutNoUpdate: // do we need a killing state?
		return stoppedState
	case TimeoutDwell: // do we need a killing state?
		return failedState
	default:
		panic("unreachable")
	}
}

func stoppedState(t transition) stateFn {
	switch t {
	case ApiCreate:
		return stoppedState
	case ApiStart:
		return startingState
	case ApiStop:
		return stoppedState
	case ApiDestroy:
		return destroyedState
	default:
		panic("unreachable")
	}
}

func finishedState(t transition) stateFn {
	switch t {
	case ApiCreate:
		return finishedState
	case ApiStart:
		return startingState
	case ApiStop:
		return finishedState
	case ApiDestroy:
		return stoppingState
	case ConfigAlwaysRestart:
		return restartWaitState
	default:
		panic("unreachable")
	}
}

func failedState(t transition) stateFn {
	switch t {
	case ApiCreate:
		return failedState
	case ApiStart:
		return startingState
	case ApiStop:
		return failedState
	case ApiDestroy:
		return stoppingState
	case ConfigAlwaysRestart:
		return restartWaitState
	case ConfigOnFailureRestart:
		return restartWaitState
	case SupExitFail:
		return failedState
	case SupExitOK:
		return failedState
	case SupSignal:
		return failedState
	case SupExitOOM:
		return failedState
	case SupUp:
		return runningState
	case SupStarting:
		return startingState
	default:
		panic("unreachable")
	}
}

func restartWaitState(t transition) stateFn {
	switch t {
	case ApiCreate:
		return restartWaitState
	case ApiStart:
		return startingState
	case ApiStop:
		return stoppedState
	case ApiDestroy:
		return destroyedState
	case TimeoutDwell:
		return startingState
	default:
		panic("unreachable")
	}
}
