package agentrepr

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
	schedule   transition = iota // async request made; from user
	unschedule                   // async request made; from user
	created                      // from event stream
	running                      // from event stream
	stopped                      // == failed/finished; from event stream
	deleted                      // from event stream
	timeout                      // un/schedule request failed; return to previous state
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

func (t transition) String() string {
	switch t {
	case schedule:
		return "schedule"
	case unschedule:
		return "unschedule"
	case created:
		return "created"
	case running:
		return "running"
	case stopped:
		return "stopped"
	case deleted:
		return "deleted"
	case timeout:
		return "timeout"
	}
	return "(unknown)"
}

func s2t(s agent.ContainerStatus) transition {
	switch s {
	case agent.ContainerStatusCreated:
		return created
	case agent.ContainerStatusRunning:
		return running
	case agent.ContainerStatusFailed:
		return stopped
	case agent.ContainerStatusFinished:
		return stopped
	case agent.ContainerStatusDeleted:
		return deleted
	default:
		panic(fmt.Sprintf("unknown ContainerStatus %v", s))
	}
}

func initialState(t transition) stateFn {
	switch t {
	case schedule:
		return pendingScheduleState
	case unschedule:
		return nil
	case created:
		return createdState
	case running:
		return runningState
	case stopped:
		return createdState
	case deleted:
		return nil
	case timeout:
		return nil
	default:
		panic("unreachable")
	}
}

func pendingScheduleState(t transition) stateFn {
	switch t {
	case schedule:
		return pendingScheduleState
	case unschedule:
		return pendingScheduleState
	case created:
		return createdState
	case running:
		return runningState
	case stopped:
		return createdState
	case deleted:
		return nil
	case timeout:
		return nil
	default:
		panic("unreachable")
	}
}

func createdState(t transition) stateFn {
	switch t {
	case schedule:
		return createdState
	case unschedule:
		return createdPendingUnscheduleState
	case created:
		return createdState
	case running:
		return runningState
	case stopped:
		return createdState
	case deleted:
		return nil
	case timeout:
		return createdState
	default:
		panic("unreachable")
	}
}

func runningState(t transition) stateFn {
	switch t {
	case schedule:
		return runningState
	case unschedule:
		return runningPendingUnscheduleState
	case created:
		return createdState
	case running:
		return runningState
	case stopped:
		return createdState
	case deleted:
		return nil
	case timeout:
		return runningState
	default:
		panic("unreachable")
	}
}

func createdPendingUnscheduleState(t transition) stateFn {
	switch t {
	case schedule:
		return createdPendingUnscheduleState // ignore
	case unschedule:
		return createdPendingUnscheduleState // ignore
	case created:
		return createdPendingUnscheduleState // ignore
	case running:
		return runningPendingUnscheduleState // shift (make sure the timeout puts us in the right place)
	case stopped:
		return createdPendingUnscheduleState // ignore
	case deleted:
		return nil
	case timeout:
		return createdState
	default:
		panic("unreachable")
	}
}

func runningPendingUnscheduleState(t transition) stateFn {
	switch t {
	case schedule:
		return runningPendingUnscheduleState // ignore
	case unschedule:
		return runningPendingUnscheduleState // ignore
	case created:
		return createdPendingUnscheduleState // shift
	case running:
		return runningPendingUnscheduleState // ignore
	case stopped:
		return createdPendingUnscheduleState // shift
	case deleted:
		return nil
	case timeout:
		return runningState
	default:
		panic("unreachable")
	}
}
