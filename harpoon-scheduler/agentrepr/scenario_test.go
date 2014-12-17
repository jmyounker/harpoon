package agentrepr_test

import (
	"runtime"
	"testing"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-scheduler/agentrepr"
)

func TestCreatedBecomesRunning(t *testing.T) {
	// A client has a container in state created
	c := agentrepr.NewFakeClient(t, "foo", false)
	c.Create("bar", agent.ContainerConfig{})

	// A representation is built for the client
	r := agentrepr.New(c)
	runtime.Gosched()

	// The container is detected as state created
	if want, have := agent.ContainerStatusCreated, r.Snapshot()["foo"].Containers["bar"].ContainerStatus; want != have {
		t.Errorf("want %q, have %q", want, have)
	}

	// A schedule request comes
	if err := r.Schedule("bar", agent.ContainerConfig{}); err != nil {
		t.Fatal(err)
	}
	runtime.Gosched()

	// The container is detected as state running
	if want, have := agent.ContainerStatusRunning, r.Snapshot()["foo"].Containers["bar"].ContainerStatus; want != have {
		t.Errorf("want %q, have %q", want, have)
	}
}

func TestFailedBecomesCreated(t *testing.T) {
	// A client has a container in state running
	c := agentrepr.NewFakeClient(t, "foo", false)
	c.Put("bar", agent.ContainerConfig{})
	c.Start("bar")

	// A representation is built for the client
	r := agentrepr.New(c)
	runtime.Gosched()

	// The container is detected as state running
	if want, have := agent.ContainerStatusRunning, r.Snapshot()["foo"].Containers["bar"].ContainerStatus; want != have {
		t.Errorf("want %q, have %q", want, have)
	}

	// The container stops
	c.Stop("bar")
	runtime.Gosched()

	// The container is detected as state finished
	if want, have := agent.ContainerStatusFinished, r.Snapshot()["foo"].Containers["bar"].ContainerStatus; want != have {
		t.Errorf("want %q, have %q", want, have)
	}
}
