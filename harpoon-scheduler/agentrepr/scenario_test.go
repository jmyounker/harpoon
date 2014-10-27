package agentrepr_test

import (
	"testing"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-scheduler/agentrepr"
)

func TestCreatedBecomesRunning(t *testing.T) {
	// A client has a container in state created
	c := agentrepr.NewFakeClient(t, "foo")
	c.Put("bar", agent.ContainerConfig{})

	// A representation is built for the client
	r := agentrepr.New(c)
	time.Sleep(time.Millisecond)

	// The container is detected as state created
	if want, have := agent.ContainerStatusCreated, r.Snapshot()["foo"].Containers["bar"].ContainerStatus; want != have {
		t.Errorf("want %q, have %q", want, have)
	}

	// A schedule request comes
	r.Schedule("bar", agent.ContainerConfig{})
	time.Sleep(time.Millisecond)

	// The container is detected as state running
	if want, have := agent.ContainerStatusRunning, r.Snapshot()["foo"].Containers["bar"].ContainerStatus; want != have {
		t.Errorf("want %q, have %q", want, have)
	}
}

func TestFailedBecomesCreated(t *testing.T) {
	// A client has a container in state running
	c := agentrepr.NewFakeClient(t, "foo")
	c.Put("bar", agent.ContainerConfig{})
	c.Start("bar")

	// A representation is built for the client
	r := agentrepr.New(c)
	time.Sleep(time.Millisecond)

	// The container is detected as state running
	if want, have := agent.ContainerStatusRunning, r.Snapshot()["foo"].Containers["bar"].ContainerStatus; want != have {
		t.Errorf("want %q, have %q", want, have)
	}

	// The container stops
	c.Stop("bar")
	time.Sleep(time.Millisecond)

	// The container is detected as state finished
	if want, have := agent.ContainerStatusFinished, r.Snapshot()["foo"].Containers["bar"].ContainerStatus; want != have {
		t.Errorf("want %q, have %q", want, have)
	}
}
