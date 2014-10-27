package reprproxy

import (
	"io/ioutil"
	"log"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"

	"testing"
	"time"
)

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

func TestAgents(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	NewAgentRepresentation = NewFakeRepr

	var (
		a       = newAgents()
		updatec = make(chan map[string]agent.StateEvent, 1)
	)

	if want, have := 0, len(a.endpoints()); want != have {
		t.Errorf("want %d, have %d", want, have)
	}

	a.update([]string{"foo"}, updatec)
	time.Sleep(time.Millisecond)

	select {
	case <-updatec:
	default:
		t.Errorf("no update received")
	}

	if want, have := 1, len(a.endpoints()); want != have {
		t.Errorf("want %d, have %d", want, have)
	}

	repr, ok := a.get("foo")

	if !ok {
		t.Errorf("didn't get representation")
	}

	if want, have := "foo", repr.Endpoint(); want != have {
		t.Errorf("want %q, have %q", want, have)
	}

	a.update([]string{}, updatec) // clear

	select {
	case <-updatec:
		t.Errorf("erroneous update received")
	default:
	}

	if want, have := 0, len(a.endpoints()); want != have {
		t.Errorf("want %d, have %d", want, have)
	}
}

func TestState(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	s := newState()

	s.update(map[string]agent.StateEvent{
		"foo": agent.StateEvent{Containers: map[string]agent.ContainerInstance{
			"bar": agent.ContainerInstance{ContainerStatus: agent.ContainerStatusFinished},
		}},
	})

	if want, have := agent.ContainerStatusFinished, s.copy()["foo"].Containers["bar"].ContainerStatus; want != have {
		t.Errorf("want %q, have %q", want, have)
	}

	s.synchronize([]string{"foo", "quux"}) // no effect

	if want, have := agent.ContainerStatusFinished, s.copy()["foo"].Containers["bar"].ContainerStatus; want != have {
		t.Errorf("want %q, have %q", want, have)
	}

	s.synchronize([]string{"quux"}) // drop foo = drop its ContainerInstances

	if want, have := 0, len(s.copy()); want != have {
		t.Errorf("want %d, have %d", want, have)
	}
}
