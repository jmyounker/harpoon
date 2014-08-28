package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestShepherdBasicFunctionality(t *testing.T) {
	log.SetOutput(ioutil.Discard)
	//log.SetFlags(log.Lmicroseconds)

	const numAgents = 9 // minimum 2

	var (
		agents  = make([]*agent.Mock, numAgents)
		servers = make([]*httptest.Server, numAgents)
		clients = make([]agent.Agent, numAgents)
	)

	// Start all the agents, servers, and clients.

	for i := 0; i < numAgents; i++ {
		agents[i] = agent.NewMock()

		servers[i] = httptest.NewServer(agents[i])
		defer servers[i].Close()
		defer servers[i].CloseClientConnections()

		client, err := agent.NewClient(servers[i].URL)
		if err != nil {
			t.Fatal(err)
		}

		clients[i] = client
	}

	// Start the shepherd and setup all the subscriptions.

	var (
		discovery = newManualAgentDiscovery([]string{servers[0].URL}) // start with one
		shepherd  = newRealShepherd(discovery)
		updatec   = make(chan map[string]map[string]agent.ContainerInstance)
		requestc  = make(chan map[string]map[string]agent.ContainerInstance)
	)

	go func() {
		current, ok := <-updatec
		if !ok {
			return
		}

		for {
			select {
			case requestc <- current:
			case current, ok = <-updatec:
				if !ok {
					return
				}
			}
		}
	}()

	shepherd.subscribe(updatec)
	defer close(updatec)
	defer shepherd.unsubscribe(updatec)

	// The shepherd starts with one empty state machine.

	if want, have := 1, shepherd.size(); want != have {
		t.Fatalf("want %d, have %d", want, have)
	}

	current := <-requestc

	if _, ok := current[servers[0].URL]; !ok {
		t.Fatalf("%s not represented", servers[0].URL)
	}

	if want, have := 0, len(current[servers[0].URL]); want != have {
		t.Fatalf("%s: want %d, have %d", servers[0].URL, want, have)
	}

	// Schedule a container on that agent.

	if err := clients[0].Put("first-container", agent.ContainerConfig{}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)

	// Verify the shepherd told us about it.

	if err := verifyInstances(t, <-requestc,
		servers[0].URL, "first-container", agent.ContainerStatusRunning,
	); err != nil {
		t.Fatal(err)
	}

	// Schedule another container on that agent.

	if err := clients[0].Put("second-container", agent.ContainerConfig{}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)

	// Verify the shepherd told us about it.

	if err := verifyInstances(t, <-requestc,
		servers[0].URL, "first-container", agent.ContainerStatusRunning,
		servers[0].URL, "second-container", agent.ContainerStatusRunning,
	); err != nil {
		t.Fatal(err)
	}

	// Stop the first container.

	if err := clients[0].Stop("first-container"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)

	// Verify the shepherd told us about it.

	if err := verifyInstances(t, <-requestc,
		servers[0].URL, "first-container", agent.ContainerStatusFinished,
		servers[0].URL, "second-container", agent.ContainerStatusRunning,
	); err != nil {
		t.Fatal(err)
	}

	// Schedule a bunch more containers on the other agents.

	for i := 1; i < numAgents; i++ {
		if err := clients[i].Put(fmt.Sprintf("container-%d", i), agent.ContainerConfig{}); err != nil {
			t.Fatal(err)
		}
	}

	// Add those agents to our agent discovery.

	for i := 1; i < numAgents; i++ {
		discovery.add(servers[i].URL)
	}
	time.Sleep(10 * time.Millisecond) // more time for this to propagate

	// The shepherd should detect the new agents.

	if want, have := numAgents, shepherd.size(); want != have {
		t.Fatalf("want %d, have %d", want, have)
	}

	// The overall state should reflect the new containers.

	s := []string{
		servers[0].URL, "first-container", agent.ContainerStatusFinished,
		servers[0].URL, "second-container", agent.ContainerStatusRunning,
	}

	for i := 1; i < numAgents; i++ {
		s = append(s, servers[i].URL, fmt.Sprintf("container-%d", i), agent.ContainerStatusRunning)
	}

	if err := verifyInstances(t, <-requestc, s...); err != nil {
		t.Fatal(err)
	}

	// Drop the first agent from agent discovery.

	discovery.del(servers[0].URL)
	time.Sleep(time.Millisecond)

	// The shepherd should consider that agent's containers lost.

	s = s[6:]

	if err := verifyInstances(t, <-requestc, s...); err != nil {
		t.Fatal(err)
	}
}

func verifyInstances(t *testing.T, have map[string]map[string]agent.ContainerInstance, s ...string) error {
	if len(s)%3 != 0 {
		return fmt.Errorf("bad invocation of verifyInstances")
	}

	var want = map[string]map[string]string{}

	for i := 0; i < len(s); i += 3 {
		endpoint, id, status := s[i], s[i+1], s[i+2]

		if _, ok := want[endpoint]; !ok {
			want[endpoint] = map[string]string{}
		}

		want[endpoint][id] = status
	}

	for endpoint, statuses := range want {
		instances, ok := have[endpoint]
		if !ok {
			return fmt.Errorf("want endpoint %s, but it's missing", endpoint)
		}

		for id, status := range statuses {
			instance, ok := instances[id]
			if !ok {
				return fmt.Errorf("endpoint %s, want %q, but it's missing", endpoint, id)
			}

			if want, have := status, fmt.Sprint(instance.ContainerStatus); want != have {
				return fmt.Errorf("endpoint %s, container %q: want %s, have %s", endpoint, id, want, have)
			}

			//t.Logf("%s: %s: %s OK", endpoint, id, status)
		}
	}

	return nil
}
