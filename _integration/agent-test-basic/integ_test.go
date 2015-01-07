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

type containerFixture struct {
	t      *testing.T
	id     string
	config agent.ContainerConfig
	client agent.Agent
}

func newContainerFixture(t *testing.T, id string) *containerFixture {
	cf := &containerFixture{
		t:  t,
		id: id,
	}
	cf.config = agent.ContainerConfig{
		ArtifactURL: *warheadURL,
		Command: agent.Command{
			WorkingDir: "/",
			Exec:       []string{"bin/warhead", "-listen", "0.0.0.0:$PORT_HTTP", "-log-interval", "0.1s"},
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

	client, err := agent.NewClient(*agentURL)
	if err != nil {
		t.Fatal(err)
	}
	cf.client = client
	return cf
}

func (cf *containerFixture) create(statuses ...agent.ContainerStatus) {
	statusSet := map[agent.ContainerStatus]struct{}{}
	for _, s := range statuses {
		statusSet[s] = struct{}{}
	}

	wc := cf.client.Wait(cf.id, statusSet, 20*time.Second)
	if err := cf.client.Create(cf.id, cf.config); err != nil {
		cf.t.Fatal(err)
	}
	w := <-wc
	if w.Err != nil {
		cf.t.Fatalf("Create failed: %s", w.Err)
	}
}

func (cf *containerFixture) start(statuses ...agent.ContainerStatus) {
	statusSet := map[agent.ContainerStatus]struct{}{}
	for _, s := range statuses {
		statusSet[s] = struct{}{}
	}

	wc := cf.client.Wait(cf.id, statusSet, 2*time.Second)
	if err := cf.client.Start(cf.id); err != nil {
		cf.t.Fatal(err)
	}
	w := <-wc
	if w.Err != nil {
		cf.t.Fatalf("Start failed: %s", w.Err)
	}
}

func (cf *containerFixture) stop(statuses ...agent.ContainerStatus) {
	statusSet := map[agent.ContainerStatus]struct{}{}
	for _, s := range statuses {
		statusSet[s] = struct{}{}
	}

	wc := cf.client.Wait(cf.id, statusSet, 2*time.Second)
	if err := cf.client.Stop(cf.id); err != nil {
		cf.t.Fatal(err)
	}
	w := <-wc
	if w.Err != nil {
		cf.t.Fatalf("Stop failed: %s", w.Err)
	}
}

func (cf *containerFixture) destroy() {
	if err := cf.client.Destroy(cf.id); err != nil {
		cf.t.Fatal(err)
	}
}

func (cf *containerFixture) watchEvents() func() {
	c, err := agent.NewClient(*agentURL)
	if err != nil {
		panic("can't connect to agent to watch events")
	}

	events, stopper, err := c.Events()
	if err != nil {
		panic("can't watch events")
	}

	stopc := make(chan struct{})

	go func() {
		for {
			select {
			case event := <-events:
				_, ok := event.Containers[cf.id]
				if !ok {
					continue
				}
				p, err := json.MarshalIndent(event, "", "    ")
				if err == nil {
					fmt.Printf("EVENT: %s\n", string(p))
				} else {
					fmt.Printf("EVENT: %#v\n", event)
				}

			case <-stopc:
				stopper.Stop()
				return
			}
		}
	}()

	return func() {
		stopc <- struct{}{}
	}
}

func (cf *containerFixture) watchLogs() func() {
	c, err := agent.NewClient(*agentURL)
	if err != nil {
		panic("can't connect to agent to watch events")
	}

	logs, _, err := c.Log(cf.id, 10)
	if err != nil {
		panic("can't watch logs")
	}

	stopc := make(chan struct{})

	go func() {
		for {
			select {
			case line := <-logs:
				if line == "" {
					continue
				}
				fmt.Printf("LOG: %s", line)
			case <-stopc:
				return
			}
		}
	}()

	return func() {
		stopc <- struct{}{}
	}
}
