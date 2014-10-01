package main

import (
	"fmt"
	"log"
	"math/rand"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

// We need every value in `current` to be an agentState, which should include
// HostResources, probably a LastUpdatedAt, and maybe a dirty bit.

type agentState struct {
	instances map[string]agent.ContainerInstance
	resources freeResources
	dirty     bool
}

type freeResources struct {
	memory  float64
	cpus    float64
	volumes map[string]struct{}
}

func makeContainerID(cfg configstore.JobConfig, i int) string {
	return fmt.Sprintf("%s-%d", cfg.Hash(), i)
}

// randomChoice is a dumb scheduling algorithm.
//
//  cfgs:    container ID -> ContainerConfig - should be scheduled
//  states:  endpoint -> agentState          - candidate agents
//  pending: container ID -> pendingTask     - already scheduled tasks in pending state
//
//  return:
//		mapping:     endpoint -> (container ID -> ContainerConfig)
//		unscheduled: container ID -> ContainerConfig
//
func randomChoice(
	cfgs map[string]agent.ContainerConfig,
	states map[string]agentState,
	pending map[string]pendingTask,
) (
	map[string]map[string]agent.ContainerConfig,
	map[string]agent.ContainerConfig,
) {
	var mapping = map[string]map[string]agent.ContainerConfig{}

	if len(states) <= 0 {
		return mapping, cfgs
	}

	var (
		endpoints   = make([]string, 0, len(states))
		unscheduled = map[string]agent.ContainerConfig{}
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

	return mapping, unscheduled
}

// randomFit is a naïve scheduling algorithm which schedules configurations
// on a random agent that matches the requirements.
//
//  cfgs:    container ID -> ContainerConfig - should be scheduled
//  states:  endpoint -> agentState          - candidate agents
//  pending: container ID -> pendingTask     - already scheduled tasks in pending state
//
//  return:
//		mapping:     endpoint -> (container ID -> ContainerConfig)
//		unscheduled: container ID -> ContainerConfig
//
func randomFit(
	cfgs map[string]agent.ContainerConfig,
	states map[string]agentState,
	pending map[string]pendingTask,
) (
	map[string]map[string]agent.ContainerConfig,
	map[string]agent.ContainerConfig,
) {
	var (
		mapping     = map[string]map[string]agent.ContainerConfig{}
		unscheduled = map[string]agent.ContainerConfig{}
		resources   = make(map[string]freeResources, len(states))
	)

	for id, state := range states {
		resources[id] = state.resources
	}

	for _, task := range pending {
		resource, ok := resources[task.endpoint]
		if !ok {
			log.Println("Agent disappeared", task.endpoint)
			continue
		}

		resource.cpus -= task.cfg.CPUs
		resource.memory -= float64(task.cfg.Memory)
		resources[task.endpoint] = resource
	}

	for id, cfg := range cfgs {
		validEndpoints := filter(cfg, resources)

		if len(validEndpoints) <= 0 {
			unscheduled[id] = cfg
			continue
		}
		endpoint := validEndpoints[rand.Intn(len(validEndpoints))]

		placed, ok := mapping[endpoint]
		if !ok {
			placed = map[string]agent.ContainerConfig{}
		}

		placed[id] = cfg

		mapping[endpoint] = placed

		hostResources := resources[endpoint]
		hostResources.cpus -= cfg.CPUs
		hostResources.memory -= float64(cfg.Memory)
		resources[endpoint] = hostResources
	}

	return mapping, unscheduled
}

func toSet(array []string) map[string]struct{} {
	var set = map[string]struct{}{}
	for _, element := range array {
		set[element] = struct{}{}
	}
	return set
}

func filter(cfg agent.ContainerConfig, free map[string]freeResources) []string {
	var validEndpoints = make([]string, 0, len(free))

	for endpoint, resources := range free {
		if !match(cfg, resources) {
			continue
		}

		validEndpoints = append(validEndpoints, endpoint)
	}

	return validEndpoints
}

func match(cfg agent.ContainerConfig, resources freeResources) bool {
	//TODO(elena) take care about ports
	if cfg.CPUs > resources.cpus {
		return false
	}

	if float64(cfg.Memory) > resources.memory {
		return false
	}

	for volume := range cfg.Volumes {
		if _, ok := resources.volumes[volume]; !ok {
			return false
		}
	}

	return true
}
