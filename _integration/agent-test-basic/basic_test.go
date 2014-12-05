package agent_test

import (
	"flag"
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
			ArtifactURL: "http://asset-host.test/busybox.tar.gz",
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

	client, err := agent.NewClient(*agentURL)
	if err != nil {
		t.Fatal(err)
	}

	// Create container
	statuses := map[agent.ContainerStatus]struct{}{
		agent.ContainerStatusCreated: struct{}{},
	}
	wc := client.Wait("basic-test", statuses, time.Second)
	if err := client.Put("basic-test", config); err != nil {
		t.Fatal(err)
	}
	w := <-wc
	if w.Err != nil {
		t.Fatal(w.Err)
	}

	// Start container.  When successful the container will transition
	// from created->running->finished.
	statuses = map[agent.ContainerStatus]struct{}{
		agent.ContainerStatusFinished: struct{}{},
	}
	wc = client.Wait("basic-test", statuses, 2*time.Second)
	if err := client.Start("basic-test"); err != nil {
		t.Fatal(err)
	}
	w = <-wc
	if w.Err != nil {
		t.Fatalf("Never reached finished state: %s", w.Err)
	}

	// Destroy container
	if err := client.Destroy("basic-test"); err != nil {
		t.Fatal(err)
	}

	// Verify that it's gone.
	if _, err := client.Get("basic-test"); err != agent.ErrContainerNotExist {
		t.Fatal(err)
	}
}
