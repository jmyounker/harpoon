package main

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

func TestScheduleUnschedule(t *testing.T) {

	var (
		numAgents = 3
		agents    = make([]*agent.Mock, numAgents)
		servers   = make([]*httptest.Server, numAgents)
		clients   = make([]agent.Agent, numAgents)
		endpoints = make([]string, numAgents)
	)

	for i := 0; i < numAgents; i++ {
		agents[i] = agent.NewMock()

		servers[i] = httptest.NewServer(agents[i])
		defer servers[i].Close()
		defer servers[i].CloseClientConnections()

		client, err := agent.NewClient(servers[i].URL)
		if err != nil {
			t.Errorf("%v", err)
		}

		clients[i] = client
		endpoints[i] = servers[i].URL
	}

	var (
		discovery   = newManualAgentDiscovery(endpoints)
		shepherd    = newRealShepherd(discovery)
		registry    = newRealRegistry("")
		transformer = newTransformer(shepherd, registry, shepherd, time.Minute)
	)

	defer registry.quit()
	defer shepherd.quit()
	defer transformer.quit()

	ch := make(chan struct{})
	cfg := agent.ContainerConfig{Grace: agent.Grace{Shutdown: agent.JSONDuration{Duration: time.Second}}}
	jobs := []configstore.JobConfig{
		configstore.JobConfig{Job: "once", Scale: 1, ContainerConfig: cfg},
		configstore.JobConfig{Job: "twice", Scale: 2, ContainerConfig: cfg},
		configstore.JobConfig{Job: "fiveX", Scale: 5, ContainerConfig: cfg},
	}

	go func() {
		for _, job := range jobs {
			registry.schedule(job)
		}
		ch <- struct{}{}
	}()

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("could not schedule jobs in time")
	}

	time.Sleep(time.Second)

	for _, job := range jobs {
		for i := 0; i < job.Scale; i++ {
			id := makeContainerID(job, i)
			found := false
			for _, client := range clients {
				if _, err := client.Get(id); err == nil {
					found = true
					continue
				}
			}
			if !found {
				t.Fatalf("container %q not found", id)
			}
		}
	}

	go func() {
		for _, job := range jobs {
			registry.unschedule(job)
		}
		ch <- struct{}{}
	}()

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("could not unschedule job intime")
	}

	errc := make(chan error)
	go func() {
		for _, job := range jobs {
			for i := 0; i < job.Scale; i++ {
				id := makeContainerID(job, i)
				for deleted := false; !deleted; {
					for _, client := range clients {
						_, err := client.Get(id)
						if err == nil {
							continue
						}

						if err != agent.ErrContainerNotExist {
							errc <- fmt.Errorf("found %s error: %v", id, err)
							return
						}

						deleted = true
					}
				}
			}
		}
		errc <- nil
	}()

	select {
	case err := <-errc:
		if err != nil {
			t.Fatal("unschedule job intime", err)
		}
	case <-time.After(time.Second * 13):
		t.Fatalf("could not unschedule job intime")
	}
}
