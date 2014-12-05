package agent_test

import (
	"flag"
	"log"
	"testing"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

var (
	agentURL = flag.String("integ.agent.url", "", "integration test URL")
)

func TestAgent(t *testing.T) {
	var (
		config = agent.ContainerConfig{
			ArtifactURL: "http://asset-host.test/completely_bogus.tgz",
			Command: agent.Command{
				WorkingDir: "/bin",
				Exec:       []string{"./true"},
			},
			Resources: agent.Resources{
				Mem: 32,
				CPU: 1,
			},
			Restart: "no",
		}
	)

	id := "stillborn-instance"
	client, err := agent.NewClient(*agentURL)
	if err != nil {
		t.Fatal(err)
	}

	wanted := map[agent.ContainerStatus]struct{}{
		agent.ContainerStatusDeleted: struct{}{},
		agent.ContainerStatusCreated: struct{}{},
	}
	wc := client.Wait(id, wanted, 30*time.Second)

	if err := client.Put(id, config); err != nil {
		t.Fatalf("Initial creation failed: %s", err)
	}

	w := <-wc

	if w.Err != nil {
		log.Fatalf("Wait results failed: %s", w.Err)
	}

}
