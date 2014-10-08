package agent_test

import (
	"testing"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
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
				Memory: 32,
				CPUs:   1,
			},
		}
	)

	client, err := agent.NewClient("http://localhost:7777")
	if err != nil {
		t.Fatal(err)
	}

	if err := client.Put("basic-test", config); err != nil {
		t.Fatal(err)
	}

	if _, err := client.Get("basic-test"); err != nil {
		t.Fatal(err)
	}

	statuses := map[agent.ContainerStatus]struct{}{
		agent.ContainerStatusRunning:  struct{}{},
		agent.ContainerStatusFinished: struct{}{},
		agent.ContainerStatusFailed:   struct{}{},
	}

	status, err := client.Wait("basic-test", statuses, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if status == agent.ContainerStatusFailed {
		t.Fatal("container failed")
	}

	if err := client.Delete("basic-test"); err != nil {
		t.Fatal(err)
	}

	if _, err := client.Get("basic-test"); err != agent.ErrContainerNotExist {
		t.Fatal(err)
	}
}
