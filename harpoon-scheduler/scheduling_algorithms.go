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
//  cfgs:   container ID -> ContainerConfig - should be scheduled
//  states: endpoint -> agentState          - candidate agents
//
//  return:
//		mapping: endpoint -> (container ID -> ContainerConfig)
//		unscheduledCfg: container ID -> ContainerConfig
func randomChoice(cfgs map[string]agent.ContainerConfig, states map[string]agentState) (map[string]map[string]agent.ContainerConfig, map[string]agent.ContainerConfig) {
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
//  cfgs:   container ID -> ContainerConfig - should be scheduled
//  states: endpoint -> agentState          - candidate agents
//
//  return:
//		mapping: endpoint -> (container ID -> ContainerConfig)
//		unscheduledCfg: container ID -> ContainerConfig
func randomFit(cfgs map[string]agent.ContainerConfig, states map[string]agentState) (map[string]map[string]agent.ContainerConfig, map[string]agent.ContainerConfig) {
	var (
		mapping     = map[string]map[string]agent.ContainerConfig{}
		resources   = calculateFreeResources(states)
		unscheduled = map[string]agent.ContainerConfig{}
	)

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

		endpointResources := resources[endpoint]
		endpointResources.cpus -= cfg.CPUs
		endpointResources.memory -= float64(cfg.Memory)
		resources[endpoint] = endpointResources
	}

	return mapping, unscheduled
}

func calculateFreeResources(states map[string]agentState) map[string]freeResources {
	var resources = map[string]freeResources{}
	for endpoint, state := range states {
		resources[endpoint] = freeResources{
			cpus:    state.resources.CPUs.Total - state.resources.CPUs.Reserved,
			memory:  state.resources.Memory.Total - state.resources.Memory.Reserved,
			volumes: toSet(state.resources.Volumes),
		}
	}
	return resources
}

func toSet(array []string) map[string]struct{} {
	var set = map[string]struct{}{}
	for _, element := range array {
		set[element] = struct{}{}
	}
	return set
}

func filter(cfg agent.ContainerConfig, resources map[string]freeResources) []string {
	var validEndpoints = make([]string, 0, len(resources))

	for endpoint, resource := range resources {
		if !match(cfg, resource) {
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
		_, ok := resources.volumes[volume]
		if !ok {
			return false
		}
	}

	return true
}
