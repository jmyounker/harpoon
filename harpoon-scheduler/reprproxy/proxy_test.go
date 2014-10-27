package reprproxy_test

import (
	"testing"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-scheduler/reprproxy"
)

func TestSubscribeUnsubscribe(t *testing.T) {
	reprproxy.NewAgentRepresentation = reprproxy.NewFakeRepr

	var (
		d = newManualAgentDiscovery()
		p = reprproxy.New(d)
		c = make(chan map[string]agent.StateEvent, 1)
	)

	p.Subscribe(c)

	if want, have := 0, len(<-c); want != have {
		t.Errorf("want %d, have %d", want, have)
	}

	d.add("foo")

	if want, have := 1, len(<-c); want != have {
		t.Errorf("want %d, have %d", want, have)
	}

	d.add("bar")

	if want, have := 2, len(<-c); want != have {
		t.Errorf("want %d, have %d", want, have)
	}

	p.Unsubscribe(c)

	if _, ok := <-c; ok { // get the close(c)
		t.Error("got erroneous signal after Unsubscribe")
	}

	d.add("baz")

	if want, have := 0, len(<-c); want != have {
		t.Errorf("want %d, have %d", want, have)
	}
}

func TestScheduleUnschedule(t *testing.T) {
	reprproxy.NewAgentRepresentation = reprproxy.NewFakeRepr

	var (
		d = newManualAgentDiscovery("foo")
		p = reprproxy.New(d)
	)

	time.Sleep(time.Millisecond) // allow agents to be updated

	if err := p.Schedule("unknown-endpoint", "bar", agent.ContainerConfig{}); err == nil {
		t.Errorf("wanted error, got none")
	}

	p.Schedule("foo", "bar", agent.ContainerConfig{})
	time.Sleep(time.Millisecond) // allow request loop to receive created + start events

	if want, have := agent.ContainerStatusRunning, p.Snapshot()["foo"].Containers["bar"].ContainerStatus; want != have {
		t.Errorf("want %q, have %q", want, have)
	}

	if err := p.Unschedule("foo", "unknown-container"); err == nil {
		t.Errorf("wanted error, got none")
	}

	p.Unschedule("foo", "bar")
	time.Sleep(time.Millisecond) // allow request loop to receive stopped + deleted events

	if want, have := 0, len(p.Snapshot()["foo"].Containers); want != have {
		t.Errorf("want %d, have %d", want, have)
	}
}
