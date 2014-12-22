package agent_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

var (
	agentURL   = flag.String("integ.agent.url", "", "integration test URL")
	warheadURL = flag.String("integ.warhead.url", "", "integration test container URL")
)

func watchEvents(agentURL string, id string) chan struct{} {
	c, err := agent.NewClient(agentURL)
	if err != nil {
		panic("can't connect to agent to watch events")
	}

	events, stopper, err := c.Events()
	if err != nil {
		panic("can't watch events")
	}

	stop := make(chan struct{})

	go func() {
		for {
			select {
			case event := <-events:
				_, ok := event.Containers[id]
				if !ok {
					continue
				}
				p, err := json.MarshalIndent(event, "", "    ")
				if err == nil {
					fmt.Printf("EVENT: %s\n", string(p))
				} else {
					fmt.Printf("EVENT: %#v\n", event)
				}

			case <-stop:
				stopper.Stop()
				return
			}
		}
	}()

	return stop
}

func watchLogs(agentURL string, id string) chan struct{} {
	c, err := agent.NewClient(agentURL)
	if err != nil {
		panic("can't connect to agent to watch events")
	}

	logs, stopper, err := c.Log(id, 1)
	if err != nil {
		panic("can't watch logs")
	}

	stop := make(chan struct{})

	go func() {
		for {
			select {
			case log := <-logs:
				if log == "" {
					continue
				}
				fmt.Printf("LOG: %s\n", log)
			case <-stop:
				stopper.Stop()
				return
			}
		}
	}()

	return stop
}

func TestAgent(t *testing.T) {
	var (
		config = agent.ContainerConfig{
			ArtifactURL: *warheadURL,
			Command: agent.Command{
				WorkingDir: "/",
				Exec:       []string{"bin/warhead", "-listen", "0.0.0.0:$PORT_HTTP"},
			},
			Resources: agent.Resources{
				Mem: 32,
				CPU: 1,
			},
			Ports: map[string]uint16{
				"HTTP": 0,
			},
			Restart: "no",
		}
	)

	cid := "basic-test"
	client, err := agent.NewClient(*agentURL)
	if err != nil {
		t.Fatal(err)
	}

	stopec := watchEvents(*agentURL, cid)
	defer func() { stopec <- struct{}{} }()

	// Create container
	statuses := map[agent.ContainerStatus]struct{}{
		agent.ContainerStatusCreated: struct{}{},
	}
	wc := client.Wait(cid, statuses, 20*time.Second)
	if err := client.Create(cid, config); err != nil {
		t.Fatal(err)
	}
	w := <-wc
	if w.Err != nil {
		t.Fatal(w.Err)
	}

	stoplc := watchLogs(*agentURL, cid)
	defer func() { stoplc <- struct{}{} }()

	// Start container.  When successful the container will transition from created->running.
	statuses = map[agent.ContainerStatus]struct{}{
		agent.ContainerStatusRunning: struct{}{},
	}
	wc = client.Wait(cid, statuses, 2*time.Second)
	if err := client.Start(cid); err != nil {
		t.Fatal(err)
	}
	w = <-wc
	if w.Err != nil {
		t.Fatalf("Never reached running state: %s", w.Err)
	}

	// Stop container.  When successful the container will transition from created->running.
	statuses = map[agent.ContainerStatus]struct{}{
		agent.ContainerStatusFinished: struct{}{},
	}
	wc = client.Wait(cid, statuses, 10*time.Second)
	if err := client.Stop(cid); err != nil {
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
