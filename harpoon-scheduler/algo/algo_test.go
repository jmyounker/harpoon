package algo_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-scheduler/algo"
)

func TestRandomFit(t *testing.T) {
	testRandomFit(t, agent.ContainerConfig{Resources: agent.Resources{Memory: 2048}}, map[string]algo.PendingTask{}, "beefy.net")
	testRandomFit(t, agent.ContainerConfig{Resources: agent.Resources{Memory: 99999}}, map[string]algo.PendingTask{})
	testRandomFit(t, agent.ContainerConfig{Storage: agent.Storage{Volumes: map[string]string{"/container/path": "/data/wimpy"}}}, map[string]algo.PendingTask{}, "wimpy.net")
	testRandomFit(t, agent.ContainerConfig{Storage: agent.Storage{Volumes: map[string]string{"/container/path": "/data/beefy"}}}, map[string]algo.PendingTask{}, "beefy.net")
	testRandomFit(t, agent.ContainerConfig{Storage: agent.Storage{Volumes: map[string]string{"/b": "/data/beefy", "/w": "/data/wimpy"}}}, map[string]algo.PendingTask{})
	testRandomFit(t, agent.ContainerConfig{Resources: agent.Resources{CPUs: 2}}, map[string]algo.PendingTask{"": algo.PendingTask{Schedule: true, Endpoint: "beefy.net", ContainerConfig: agent.ContainerConfig{Resources: agent.Resources{CPUs: 30}}}})
}

func testRandomFit(t *testing.T, c agent.ContainerConfig, pending map[string]algo.PendingTask, want ...string) {
	testAgents := map[string]agent.StateEvent{
		"beefy.net": agent.StateEvent{
			Resources: agent.HostResources{
				Memory:  agent.TotalReservedInt{Total: 32000, Reserved: 128}, // 32GB total, 128MB reserved
				CPUs:    agent.TotalReserved{Total: 32.0, Reserved: 1.0},     // 32 CPUs total, 1 CPU reserved
				Storage: agent.TotalReserved{Total: 250 * 1e10, Reserved: 0}, // 250GB total, 0 bytes reserved
				Volumes: []string{"/data/shared", "/data/beefy"},
			},
			Containers: map[string]agent.ContainerInstance{},
		},
		"wimpy.net": agent.StateEvent{
			Resources: agent.HostResources{
				Memory:  agent.TotalReservedInt{Total: 1024, Reserved: 512},          // 1GB total, 512MB reserved
				CPUs:    agent.TotalReserved{Total: 4.0, Reserved: 3.0},              // 4 CPUs total, 3 CPUs reserved
				Storage: agent.TotalReserved{Total: 100 * 1e10, Reserved: 70 * 1e10}, // 100GB total, 70GB reserved
				Volumes: []string{"/data/shared", "/data/wimpy"},
			},
			Containers: map[string]agent.ContainerInstance{},
		},
	}

	sort.StringSlice(want).Sort()

	matched, _ := algo.RandomFit(map[string]agent.ContainerConfig{"foo": c}, testAgents, pending)
	if len(want) != len(matched) {
		t.Errorf("want %d, have %d", len(want), len(matched))
	}

	have := sort.StringSlice{}
	for s := range matched {
		have = append(have, s)
	}
	have.Sort()

	if fmt.Sprint(want) != fmt.Sprint(have) {
		t.Errorf("%v/%v: want %v, have %v", c.Resources, c.Storage.Volumes, want, have)
	}
}

func TestLeastUsed(t *testing.T) {
	var (
		testAgents = map[string]agent.StateEvent{
			"beefy.net": agent.StateEvent{
				Resources: agent.HostResources{
					Memory:  agent.TotalReservedInt{Total: 32000, Reserved: 128}, // 32GB total, 128MB reserved
					CPUs:    agent.TotalReserved{Total: 32.0, Reserved: 1.0},     // 32 CPUs total, 1 CPU reserved
					Storage: agent.TotalReserved{Total: 250 * 1e10, Reserved: 0}, // 250GB total, 0 bytes reserved
					Volumes: []string{"/data/shared", "/data/beefy"},
				},
				Containers: map[string]agent.ContainerInstance{},
			},
			"wimpy.net": agent.StateEvent{
				Resources: agent.HostResources{
					Memory:  agent.TotalReservedInt{Total: 32000, Reserved: 128}, // 32GB total, 128MB reserved
					CPUs:    agent.TotalReserved{Total: 32.0, Reserved: 1.0},     // 32 CPUs total, 1 CPU reserved
					Storage: agent.TotalReserved{Total: 250 * 1e10, Reserved: 0}, // 250GB total, 0 bytes reserved
					Volumes: []string{"/data/shared", "/data/beefy"},
				},
				Containers: map[string]agent.ContainerInstance{},
			},
		}
		cfgs = map[string]agent.ContainerConfig{
			"cfg-0": agent.ContainerConfig{Resources: agent.Resources{CPUs: 0.5}},
			"cfg-1": agent.ContainerConfig{Resources: agent.Resources{CPUs: 0.5}},
			"cfg-2": agent.ContainerConfig{Resources: agent.Resources{CPUs: 0.5}},
			"cfg-3": agent.ContainerConfig{Resources: agent.Resources{CPUs: 0.5}},
		}
	)

	matched, _ := algo.LeastUsed(cfgs, testAgents, map[string]algo.PendingTask{})
	if len(testAgents) != len(matched) {
		t.Errorf("want %d, have %d", len(testAgents), len(matched))
	}

	if want, have := 2, len(matched["beefy.net"]); want != have {
		t.Errorf("incorrect spreading of configs over agent want %d, have %d", want, have)
	}

	if want, have := 2, len(matched["wimpy.net"]); want != have {
		t.Errorf("incorrect spreading of configs over agent want %d, have %d", want, have)
	}

	if want, have := 1.0, testAgents["beefy.net"].Resources.CPUs.Reserved; want != have {
		t.Errorf("agent resources should not be changed want %f , have %f", want, have)
	}
}
