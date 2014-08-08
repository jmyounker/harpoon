package main

import (
	"fmt"
	"math/rand"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type schedulingAlgorithm func(map[string]agentState, agent.ContainerConfig) (string, error)

func randomNonDirty(agentStates map[string]agentState, tgt agent.ContainerConfig) (string, error) {
	endpoints := make([]string, 0, len(agentStates))
	for key := range agentStates {
		endpoints = append(endpoints, key)
	}
	for _, index := range rand.Perm(len(endpoints)) {
		if agentStates[endpoints[index]].Dirty {
			continue
		}
		return endpoints[index], nil
	}
	return "", fmt.Errorf("no trustable agent available")
}
