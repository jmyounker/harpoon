package agentrepr

import (
	"fmt"
	"testing"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestInstances(t *testing.T) {
	i := newInstances()

	i.advanceOne("foo", schedule)

	if want, have := fmt.Sprint(stateFn(pendingScheduleState)), fmt.Sprint(i.m["foo"].stateFn); want != have {
		t.Errorf("want %s, have %s", want, have)
	}

	i.advanceMany(map[string]agent.ContainerInstance{
		"foo": agent.ContainerInstance{ContainerStatus: agent.ContainerStatusRunning},
		"bar": agent.ContainerInstance{ContainerStatus: agent.ContainerStatusFinished},
	})

	if want, have := fmt.Sprint(stateFn(runningState)), fmt.Sprint(i.m["foo"].stateFn); want != have {
		t.Errorf("want %s, have %s", want, have)
	}

	if want, have := fmt.Sprint(stateFn(createdState)), fmt.Sprint(i.m["bar"].stateFn); want != have {
		t.Errorf("want %s, have %s", want, have)
	}

	m := i.copy() // make sure we get a copy by value
	ci := m["foo"]
	ci.ContainerStatus = agent.ContainerStatusFailed
	m["foo"] = ci

	if want, have := fmt.Sprint(stateFn(runningState)), fmt.Sprint(i.m["foo"].stateFn); want != have {
		t.Errorf("want %s, have %s", want, have)
	}

	i.reset()

	if want, have := 0, len(i.m); want != have {
		t.Errorf("want %d, have %d", want, have)
	}
}

func TestResources(t *testing.T) {
	r := newResources()

	r.set(agent.HostResources{CPUs: agent.TotalReserved{Total: 32.0, Reserved: 16.0}})

	if want, have := 16.0, r.HostResources.CPUs.Reserved; want != have {
		t.Errorf("want %.2f, have %.2f", want, have)
	}

	m := r.copy()
	m.CPUs.Reserved = 17.0

	if want, have := 16.0, r.HostResources.CPUs.Reserved; want != have {
		t.Errorf("want %.2f, have %.2f", want, have)
	}

	r.reset()

	if want, have := 0.0, r.HostResources.CPUs.Reserved; want != have {
		t.Errorf("want %.2f, have %.2f", want, have)
	}
}

func TestSubscribers(t *testing.T) {
	var (
		s  = newSubscribers()
		c  = make(chan map[string]agent.StateEvent, 1)
		se map[string]agent.StateEvent
	)

	// Empty broadcast
	s.broadcast(map[string]agent.StateEvent{"foo": agent.StateEvent{}})

	select {
	case se = <-c:
	default:
	}

	if _, ok := se["foo"]; ok {
		t.Errorf("receive foo, but wasn't supposed to")
	}

	// Add a subscriber

	s.add(c)

	if want, have := 1, len(s.m); want != have {
		t.Errorf("want %d, have %d", want, have)
	}

	// Normal broadcast

	s.broadcast(map[string]agent.StateEvent{"foo": agent.StateEvent{}})

	se = map[string]agent.StateEvent{}
	select {
	case se = <-c:
	default:
	}

	if _, ok := se["foo"]; !ok {
		t.Errorf("didn't receive foo")
	}

	// Remove subscriber

	s.remove(c)

	if want, have := 0, len(s.m); want != have {
		t.Errorf("want %d, have %d", want, have)
	}

	// Another empty broadcast

	s.broadcast(map[string]agent.StateEvent{"foo": agent.StateEvent{}})

	se = map[string]agent.StateEvent{}
	select {
	case se = <-c:
	default:
	}

	if _, ok := se["foo"]; ok {
		t.Errorf("receive foo, but wasn't supposed to")
	}
}

func TestOutstanding(t *testing.T) {
	var (
		o        = newOutstanding()
		successc = make(chan string, 1)
		failurec = make(chan string, 1)
	)

	o.want("foo", agent.ContainerStatusRunning, successc, failurec)

	if want, have := true, o.contains("foo"); want != have {
		t.Errorf("want %v, have %v", want, have)
	}

	// no match

	o.signal(map[string]agent.ContainerInstance{"foo": agent.ContainerInstance{ContainerStatus: agent.ContainerStatusCreated}})
	time.Sleep(time.Millisecond)

	if want, have := true, o.contains("foo"); want != have {
		t.Errorf("want %v, have %v", want, have)
	}

	// match

	o.signal(map[string]agent.ContainerInstance{"foo": agent.ContainerInstance{ContainerStatus: agent.ContainerStatusRunning}})
	time.Sleep(100 * time.Millisecond)

	if want, have := true, o.contains("foo"); want != have {
		// will still contain until we remove
		t.Errorf("want %v, have %v", want, have)
	}

	var id string
	select {
	case id = <-successc:
	case id = <-failurec:
		t.Errorf("got failure")
	default:
		t.Errorf("got nothing")
	}

	if want, have := "foo", id; want != have {
		t.Errorf("want %q, have %q", want, have)
	}

	o.remove("foo")

	if want, have := false, o.contains("foo"); want != have {
		t.Errorf("want %v, have %v", want, have)
	}
}
