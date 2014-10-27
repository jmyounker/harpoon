package algo_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-scheduler/algo"
)

var testAgents = map[string]agent.StateEvent{
	"beefy.net": agent.StateEvent{
		Resources: agent.HostResources{
			Memory:  agent.TotalReservedInt{Total: 32000, Reserved: 128}, // 32GB total, 128MB reserved
			CPUs:    agent.TotalReserved{Total: 32.0, Reserved: 1.0},     // 32 CPUs total, 1 CPU reserved
			Storage: agent.TotalReserved{Total: 250 * 1e10, Reserved: 0}, // 250GB total, 0 bytes reserved
			Volumes: []string{"/data/shared", "/data/beefy"},
		},
	},
	"wimpy.net": agent.StateEvent{
		Resources: agent.HostResources{
			Memory:  agent.TotalReservedInt{Total: 1024, Reserved: 512},          // 1GB total, 512MB reserved
			CPUs:    agent.TotalReserved{Total: 4.0, Reserved: 3.0},              // 4 CPUs total, 3 CPUs reserved
			Storage: agent.TotalReserved{Total: 100 * 1e10, Reserved: 70 * 1e10}, // 100GB total, 70GB reserved
			Volumes: []string{"/data/shared", "/data/wimpy"},
		},
	},
}

func TestRandomFit(t *testing.T) {
	testRandomFit(t, agent.ContainerConfig{Resources: agent.Resources{Memory: 2048}}, "beefy.net")
	testRandomFit(t, agent.ContainerConfig{Resources: agent.Resources{Memory: 99999}})
	testRandomFit(t, agent.ContainerConfig{Storage: agent.Storage{Volumes: map[string]string{"/container/path": "/data/wimpy"}}}, "wimpy.net")
	testRandomFit(t, agent.ContainerConfig{Storage: agent.Storage{Volumes: map[string]string{"/container/path": "/data/beefy"}}}, "beefy.net")
	testRandomFit(t, agent.ContainerConfig{Storage: agent.Storage{Volumes: map[string]string{"/b": "/data/beefy", "/w": "/data/wimpy"}}})
}

func testRandomFit(t *testing.T, c agent.ContainerConfig, want ...string) {
	sort.StringSlice(want).Sort()

	matched, _ := algo.RandomFit(map[string]agent.ContainerConfig{"foo": c}, testAgents)
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
