package main

import (
	"fmt"
	"math/rand"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

// We need every value in `current` to be an agentState, which should include
// HostResources, probably a LastUpdatedAt, and maybe a dirty bit.

type agentState struct {
	instances map[string]agent.ContainerInstance
	resources agent.HostResources
	dirty     bool
}

func makeContainerID(cfg configstore.JobConfig, i int) string {
	return fmt.Sprintf("%s-%d", cfg.Hash(), i)
}

// randomChoice is a dumb scheduling algorithm.
//
//  cfgs:   container ID -> ContainerConfig - should be scheduled
//  states: endpoint -> agentState          - candidate agents
//
//  return: endpoint -> (container ID -> ContainerConfig)
//
func randomChoice(cfgs map[string]agent.ContainerConfig, states map[string]agentState) (map[string]map[string]agent.ContainerConfig, error) {
	if len(states) <= 0 {
		return map[string]map[string]agent.ContainerConfig{}, fmt.Errorf("no available agents")
	}

	var (
		endpoints = make([]string, 0, len(states))
		mapping   = map[string]map[string]agent.ContainerConfig{}
	)

	for endpoint := range states {
		endpoints = append(endpoints, endpoint)
	}

	for id, cfg := range cfgs {
		endpoint := endpoints[rand.Intn(len(endpoints))]

		placed, ok := mapping[endpoint]
		if !ok {
			placed = map[string]agent.ContainerConfig{}
		}

		placed[id] = cfg

		mapping[endpoint] = placed
	}

	return mapping, nil
}
