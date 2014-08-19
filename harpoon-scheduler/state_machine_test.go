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

func TestStateMachineBasicFunctionality(t *testing.T) {
	log.SetOutput(ioutil.Discard)
	//log.SetFlags(log.Lmicroseconds)

	var (
		mock    = agent.NewMock()
		server  = httptest.NewServer(mock)
		machine = newRealStateMachine(server.URL)
	)

	defer server.Close()
	defer machine.quit()

	client, err := agent.NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	var (
		id       = "foo-container"
		updatec  = make(chan map[string]map[string]agent.ContainerInstance)
		currentc = make(chan string) // always for `id`
	)

	go func() {
		var current = "initializing, unknown"
		for {
			select {
			case currentc <- current:
			case update, ok := <-updatec:
				if !ok {
					current = "shut down"
					return
				}
				current = "most recent update didn't have the instance"
				for endpoint, instances := range update {
					if endpoint == server.URL {
						for id0, instance := range instances {
							if id0 == id {
								current = fmt.Sprint(instance.Status)
							}
						}
					}
				}
			}
		}
	}()

	machine.subscribe(updatec)
	defer close(updatec)
	defer machine.unsubscribe(updatec)

	if err := client.Put(id, agent.ContainerConfig{}); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Millisecond)

	if want, have := agent.ContainerStatusRunning, <-currentc; want != have {
		t.Fatalf("want %q, have %q", want, have)
	}

	if err := client.Stop(id); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Millisecond)

	if want, have := agent.ContainerStatusFinished, <-currentc; want != have {
		t.Fatalf("want %q, have %q", want, have)
	}
}

func TestStateMachineResilience(t *testing.T) {
	// This test requires we enhance the agent client to not use the default
	// EventSource server, but manually manage the connection state and report
	// interruptions to clients.

	t.Skip("not yet implemented")

	// Create an agent with some containers
	// Add a state machine for it
	// Verify connection is live and containers are represented
	// Kill the agent
	// Verify it's been detected, but containers are still represented
	// Restart the agent with the containers
	// Verify the state machine re-establishes the connection
	// Kill it again and wait for the timeout
	// Verify containers are gone in the state machine
	// Restart the agent with the containers
	// Verify containers come back in the state machine
}
