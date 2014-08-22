package main

import (
	"fmt"
	"math/rand"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

// We need every value in `current` to be an agentState, which should include
// HostResources, probably a LastUpdatedAt, and maybe a dirty bit.

func randomChoice(cfg configstore.JobConfig, current map[string]map[string]agent.ContainerInstance) ([]taskSpec, error) {
	if len(current) <= 0 {
		return []taskSpec{}, fmt.Errorf("no available agents")
	}

	var (
		out       = []taskSpec{}
		endpoints = make([]string, 0, len(current))
	)

	for endpoint := range current {
		endpoints = append(endpoints, endpoint)
	}

	for i := 0; i < cfg.Scale; i++ {
		out = append(out, taskSpec{
			Endpoint:        endpoints[rand.Intn(len(endpoints))],
			Job:             cfg.Job,
			ContainerID:     makeContainerID(cfg, i),
			ContainerConfig: cfg.ContainerConfig,
		})
	}

	return out, nil
}
