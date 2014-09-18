package main

import (
	"reflect"
	"testing"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

var ok = struct{}{}

func TestMatch(t *testing.T) {
	var (
		cfg            = newConfig(300, 2, map[string]string{"/a": "", "/b": ""})
		validResources = []freeResources{
			freeResources{cpus: 2, memory: 300, volumes: toSet([]string{"/a", "/b", "/c"})},
			freeResources{cpus: 2, memory: 400, volumes: toSet([]string{"/a", "/b"})},
			freeResources{cpus: 3, memory: 300, volumes: toSet([]string{"/a", "/b"})},
			freeResources{cpus: 4, memory: 300, volumes: toSet([]string{"/a", "/b", "/c"})},
			freeResources{cpus: 4, memory: 300, volumes: toSet([]string{"/a", "/b", "/c"})},
		}
		invalidResources = []freeResources{
			freeResources{cpus: 1, memory: 300, volumes: toSet([]string{"/a", "/b"})},
			freeResources{cpus: 2, memory: 200, volumes: toSet([]string{"/a", "/b"})},
			freeResources{cpus: 2, memory: 200, volumes: toSet([]string{"/a", "/b"})},
			freeResources{cpus: 100, memory: 1100, volumes: toSet([]string{"/a", "/c"})},
			freeResources{cpus: 2, memory: 200, volumes: toSet([]string{"/b"})},
			freeResources{cpus: 2, memory: 200, volumes: toSet([]string{"/a"})},
			freeResources{cpus: 2, memory: 200, volumes: toSet([]string{"/a"})},
		}
	)

	for _, resources := range validResources {
		if !match(cfg, resources) {
			t.Errorf("error %v should match to the resources %v ", cfg, resources)
		}
	}

	for _, resources := range invalidResources {
		if match(cfg, resources) {
			t.Errorf("error %v should not match to the resources %v ", cfg, resources)
		}
	}

}

func TestFilter(t *testing.T) {
	var states = map[string]agentState{
		"state1": newAgentState(700, 3, []string{"/a", "/b", "/c"}),
		"state2": newAgentState(200, 11, []string{"/a", "/c"}),
		"state3": newAgentState(1, 1, []string{}),
		"state4": newAgentState(700, 3, []string{"/b"}),
	}

	free := calculateFreeResources(states)
	validAgents := filter(newConfig(1100, 12, map[string]string{}), free)
	if len(validAgents) != 0 {
		t.Errorf("found agent for config with infeasible resources")
	}

	validAgents = filter(newConfig(300, 2, map[string]string{"/a": "", "/b": ""}), free)
	if expected, actual := 1, len(validAgents); actual != expected {
		t.Fatalf("number of valid agents found: actual %d != expected %d", actual, expected)
	}
	if validAgents[0] != "state1" {
		t.Error("missing valid agent after filtering")
	}

	free["state"] = freeResources{10000, 100, map[string]struct{}{}}
	validAgents = filter(newConfig(1, 1, map[string]string{}), free)
	if expected, actual := len(free), len(validAgents); actual != expected {
		t.Fatalf("number of valid agents found: actual %d != expected %d", actual, expected)
	}

	for _, agent := range validAgents {
		if _, ok := free[agent]; !ok {
			t.Errorf("unexpected agent after filter %s", agent)
		}
		delete(free, agent)
	}
}

func TestRandomFit(t *testing.T) {
	var (
		cfgs = map[string]agent.ContainerConfig{
			"cfg1": newConfig(300, 2, map[string]string{"/a": "", "/b": ""}),
			"cfg2": newConfig(300, 3, map[string]string{"/c": "", "/b": ""}),
			"cfg3": newConfig(1, 4, map[string]string{}),
			"cfg4": newConfig(1, 4, map[string]string{}),
			"cfg5": newConfig(1, 4, map[string]string{}),
			"cfg6": newConfig(1100, 12, map[string]string{}),
		}
		states = map[string]agentState{
			"state1": newAgentState(700, 3, []string{"/a", "/b", "/c"}),
			"state2": newAgentState(200, 11, []string{"/a", "/c"}),
			"state3": newAgentState(1, 1, []string{}),
			"state4": newAgentState(700, 3, []string{"/b"}),
		}
		expectedMapping = []struct {
			name           string
			scheduledTasks int
			possibleTasks  map[string]struct{}
		}{
			{"state1", 1, map[string]struct{}{"cfg1": ok, "cfg2": ok}},
			{"state2", 2, map[string]struct{}{"cfg3": ok, "cfg4": ok, "cfg5": ok}},
		}
	)

	mapping, unscheduled := randomFit(cfgs, states)
	if len(mapping) != len(expectedMapping) {
		t.Fatalf("wrong count of agents with scheduled tasks: actual %d != expected %d", len(mapping), len(expectedMapping))
	}
	var (
		_, unscheduledCfg1 = unscheduled["cfg1"]
		_, unscheduledCfg2 = unscheduled["cfg2"]
	)
	if unscheduledCfg1 == unscheduledCfg2 {
		if unscheduledCfg1 {
			t.Fatal("configs [cfg1, cfg2] should not be both unscheduled")
		} else {
			t.Fatal("configs [cfg1, cfg2] should not be both scheduled")
		}
	}

	var (
		_, unscheduledCfg3 = unscheduled["cfg3"]
		_, unscheduledCfg4 = unscheduled["cfg4"]
		_, unscheduledCfg5 = unscheduled["cfg5"]
	)

	if !unscheduledCfg3 && !unscheduledCfg4 && !unscheduledCfg5 {
		t.Fatalf("one of the config (cfg3, cfg4, cfg5) should be unscheduled: unscheduled (%v, %v, %v)",
			unscheduledCfg3,
			unscheduledCfg4,
			unscheduledCfg5,
		)
	}

	if _, unscheduledCfg6 := unscheduled["cfg6"]; !unscheduledCfg6 {
		t.Fatalf("Task cfg6 should not be scheduled!")
	}

	state3 := "state3"
	tasks, ok := mapping[state3]
	if ok {
		t.Fatalf("agent %q should not have scheduled tasks but have %v", state3, tasks)
	}

	if expectedUnscheduledCfgs := 3; len(unscheduled) != expectedUnscheduledCfgs {
		t.Fatalf("unscheduled task count: actual %d != expected %d", len(unscheduled), expectedUnscheduledCfgs)
	}

	for _, agent := range expectedMapping {
		tasks := mapping[agent.name]
		if len(tasks) != agent.scheduledTasks {
			t.Fatalf("Wrong schedule agent: %v actual %d != expected %d", agent, len(tasks), agent.scheduledTasks)
		}
		for name, config := range tasks {
			if _, ok := agent.possibleTasks[name]; !ok {
				t.Fatalf("Task %s should not be schedule on agent %s", name, agent.name)
			}
			if !reflect.DeepEqual(config, cfgs[name]) {
				t.Fatalf("Not right configuration %s returned actual %v != expected %v", name, config, cfgs[name])
			}
		}
	}
}

func TestRandomFitWithoutResources(t *testing.T) {
	var (
		cfgs   = map[string]agent.ContainerConfig{}
		states = map[string]agentState{}
	)

	mapping, unscheduled := randomFit(cfgs, states)
	if expected, actual := 0, len(unscheduled); actual != expected {
		t.Fatalf("unscheduled task count: actual %d != expected %d", actual, expected)
	}
	if expected, actual := 0, len(mapping); actual != expected {
		t.Fatalf("empty config should not return any mapping %v", mapping)
	}

	cfgs["random1"] = newConfig(100, 12, map[string]string{"/a": ""})
	cfgs["random2"] = newConfig(100, 12, map[string]string{})

	mapping, unscheduled = randomFit(cfgs, states)
	if expected, actual := 0, len(mapping); actual != expected {
		t.Fatalf("unscheduled task count: actual %d != expected %d", actual, expected)
	}

	if !reflect.DeepEqual(cfgs, unscheduled) {
		t.Fatalf("incorrect unscheduled tasks returned")
	}
}

func newConfig(memory int, cpus float64, volumes map[string]string) agent.ContainerConfig {
	return agent.ContainerConfig{
		Resources: agent.Resources{
			Memory: memory,
			CPUs:   cpus,
		},
		Storage: agent.Storage{
			Volumes: volumes,
		},
	}
}

func newAgentState(memory float64, cpus float64, volumes []string) agentState {
	return agentState{
		resources: agent.HostResources{
			CPUs: agent.TotalReserved{
				Total: cpus,
			},
			Memory: agent.TotalReserved{
				Total: memory,
			},
			Volumes: volumes,
		},
	}
}
