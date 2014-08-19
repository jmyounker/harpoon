package main

import (
	"fmt"
	"math/rand"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

// We need every representation of `current` to be an agentState, which should
// include HostResources, probably a LastUpdatedAt, and maybe a dirty bit.

func randomChoice(jobConfig configstore.JobConfig, current map[string]map[string]agent.ContainerInstance) ([]taskSpec, error) {
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

	for _, taskConfig := range jobConfig.Tasks {
		for i := 0; i < taskConfig.Scale; i++ {
			out = append(out, taskSpec{
				Endpoint:        endpoints[rand.Intn(len(endpoints))],
				JobName:         jobConfig.JobName,
				TaskName:        taskConfig.TaskName,
				ContainerID:     makeContainerID(jobConfig, taskConfig, i),
				ContainerConfig: taskConfig.ContainerConfig,
			})
		}
	}

	return out, nil
}
