package agent_test

import (
	"testing"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestAgent(t *testing.T) {
	cf := newContainerFixture(t, "stillborn-instance")
	// Useless URL which will never load
	cf.config.ArtifactURL = "http://asset-host.testisquat/completely_bogus.tgz"
	// Creation should fail with status deleted
	cf.create(agent.ContainerStatusDeleted)
}
