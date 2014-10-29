package algo

import (
	"reflect"
	"testing"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestSortByContainersCount(t *testing.T) {
	var (
		testAgents = map[string]agent.StateEvent{
			"first.net": agent.StateEvent{
				Containers: map[string]agent.ContainerInstance{},
				Resources: agent.HostResources{
					Memory:  agent.TotalReservedInt{Total: 32000, Reserved: 128}, // 32GB total, 128MB reserved
					CPUs:    agent.TotalReserved{Total: 32.0, Reserved: 1.0},     // 32 CPUs total, 1 CPU reserved
					Storage: agent.TotalReserved{Total: 250 * 1e10, Reserved: 0}, // 250GB total, 0 bytes reserved
					Volumes: []string{"/data/shared", "/data/first"},
				},
			},
			"second.net": agent.StateEvent{
				Containers: map[string]agent.ContainerInstance{"one": agent.ContainerInstance{}},
				Resources: agent.HostResources{
					Memory:  agent.TotalReservedInt{Total: 16000, Reserved: 128}, // 32GB total, 128MB reserved
					CPUs:    agent.TotalReserved{Total: 32.0, Reserved: 1.0},     // 32 CPUs total, 1 CPU reserved
					Storage: agent.TotalReserved{Total: 250 * 1e10, Reserved: 0}, // 250GB total, 0 bytes reserved
					Volumes: []string{"/data/shared", "/data/second"},
				},
			},
			"third.net": agent.StateEvent{
				Containers: map[string]agent.ContainerInstance{"one": agent.ContainerInstance{}, "two": agent.ContainerInstance{}},
				Resources: agent.HostResources{
					Memory:  agent.TotalReservedInt{Total: 16000, Reserved: 128}, // 16GB total, 128MB reserved
					CPUs:    agent.TotalReserved{Total: 32.0, Reserved: 1.0},     // 32 CPUs total, 1 CPU reserved
					Storage: agent.TotalReserved{Total: 250 * 1e10, Reserved: 0}, // 250GB total, 0 bytes reserved
					Volumes: []string{"/data/shared", "/data/third"},
				},
			},
		}

		strategy = sortStrategy{}
		agents   = []string{"third.net", "first.net", "second.net"}
	)

	strategy.sort(agents, testAgents)

	want := []string{"first.net", "second.net", "third.net"}
	if !reflect.DeepEqual(want, agents) {
		t.Errorf("want %v, have %v", want, agents)
	}
}
