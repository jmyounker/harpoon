package agent_test

import (
	"testing"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestAgent(t *testing.T) {
	cf := newContainerFixture(t, "basic-test")

	cf.create(agent.ContainerStatusCreated)
	cf.start(agent.ContainerStatusRunning)
	cf.stop(agent.ContainerStatusFinished)
	cf.destroy()

	// Verify that it's gone.
	if _, err := cf.client.Get(cf.id); err != agent.ErrContainerNotExist {
		t.Fatal(err)
	}
}
