package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http/httptest"
	"reflect"
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
		machine = newRealStateMachine(server.URL, defaultStateMachineReconnect, defaultStateMachineAbandon)
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

func TestStateMachineInterruption(t *testing.T) {
	log.SetOutput(ioutil.Discard)
	//log.SetFlags(log.Lmicroseconds)

	// Create an agent with some container

	var (
		id     = "alfred"
		mock   = agent.NewMock()
		server = httptest.NewUnstartedServer(mock)
	)

	server.Start()
	defer server.Close()

	client := agent.MustNewClient(server.URL)
	if err := client.Put(id, agent.ContainerConfig{}); err != nil {
		t.Fatal(err)
	}

	// Create a state machine for it

	var (
		reconnect = 2 * time.Millisecond
		abandon   = 50 * time.Millisecond
	)

	machine := newRealStateMachine(server.URL, reconnect, abandon)
	defer machine.quit()
	time.Sleep(5 * time.Millisecond)

	// Verify state machine got the container state

	preSnapshot := machine.snapshot()

	if _, ok := preSnapshot[machine.endpoint()]; !ok {
		t.Fatal("machine snapshot missing its own endpoint")
	}

	if _, ok := preSnapshot[machine.endpoint()][id]; !ok {
		t.Fatal("machine snapshot missing expected container")
	}

	// Kill the agent connection

	server.CloseClientConnections()
	time.Sleep(time.Millisecond)

	// Verify state machine detected connection interruption

	if want, have := false, machine.connected(); want != have {
		t.Errorf("want %v, have %v", want, have)
	}

	// Wait for a reconnect

	ok := make(chan struct{})
	go func() {
		defer close(ok)
		for !machine.connected() {
			time.Sleep(reconnect)
		}
	}()

	select {
	case <-ok:
	case <-time.After(abandon):
		t.Fatal("state machine never reconnected")
	}

	// Verify the container state wasn't cleared

	if want, have := preSnapshot, machine.snapshot(); !reflect.DeepEqual(want, have) {
		t.Fatalf("after reconnect, want %#v, have %#v", want, have)
	}

	// Kill the agent connection kinda permanently

	server.CloseClientConnections()
	server.Close()
	server.URL = "" // needed to restart
	time.Sleep(5 * time.Millisecond)

	// Verify state machine detected connection interruption

	if want, have := false, machine.connected(); want != have {
		t.Errorf("want %v, have %v", want, have)
	}

	// Verify state machine abandons the containers after the abandon interval

	time.Sleep(2 * abandon)

	postSnapshot := machine.snapshot()

	if _, ok := postSnapshot[machine.endpoint()]; !ok {
		t.Fatal("machine snapshot missing its own endpoint")
	}

	if _, ok := postSnapshot[machine.endpoint()][id]; ok {
		t.Fatal("machine snapshot has container that should have been abandoned")
	}
}
