package main

import (
	"fmt"
	"math/rand"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

// We need every value in `current` to be an agentState, which should include
// HostResources, probably a LastUpdatedAt, and maybe a dirty bit.

func makeContainerID(cfg configstore.JobConfig, i int) string {
	return fmt.Sprintf("%s-%d", cfg.Hash(), i)
}

func randomChoice(cfg agent.ContainerConfig, current map[string]map[string]agent.ContainerInstance) (string, error) {
	if len(current) <= 0 {
		return "", fmt.Errorf("no available agents")
	}

	var endpoints = make([]string, 0, len(current))

	for endpoint := range current {
		endpoints = append(endpoints, endpoint)
	}

	return endpoints[rand.Intn(len(endpoints))], nil
}
