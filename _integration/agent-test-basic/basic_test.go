package agent_test

import (
	"fmt"
	"testing"

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

	status, err := wait(client, "basic-test")
	if err != nil {
		t.Fatal(err)
	}

	if status == agent.ContainerStatusFailed {
		t.Fatal("container failed")
	}
}

func wait(a agent.Agent, id string) (agent.ContainerStatus, error) {
	events, stopper, err := a.Events()
	if err != nil {
		return "", err
	}
	defer stopper.Stop()

	for containers := range events {
		container, ok := containers[id]
		if !ok {
			continue
		}

		switch status := container.ContainerStatus; status {
		case agent.ContainerStatusRunning, agent.ContainerStatusFailed, agent.ContainerStatusFinished:
			return status, nil
		}
	}

	return "", fmt.Errorf("event stream ended without expected status")
}
