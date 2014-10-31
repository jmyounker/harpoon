// Package algo implements scheduling algorithms.
package algo

import (
	"sort"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type sortStrategy struct {
	states          map[string]agent.StateEvent
	endpoints       []string
	containersCount int
}

func (s *sortStrategy) sort(endpoints []string, states map[string]agent.StateEvent) {
	s.endpoints = endpoints
	s.states = states

	s.containersCount = 0
	for _, state := range states {
		s.containersCount += len(state.Containers)
	}

	sort.Sort(s)
}

func (s *sortStrategy) Less(i, j int) bool {
	return s.score(i) < s.score(j)

}

func (s *sortStrategy) Len() int {
	return len(s.endpoints)
}

func (s *sortStrategy) Swap(i, j int) {
	s.endpoints[i], s.endpoints[j] = s.endpoints[j], s.endpoints[i]
}

func (s *sortStrategy) best() string {
	return s.endpoints[0]
}

func (s *sortStrategy) score(i int) float64 {
	var (
		state       = s.states[s.endpoints[i]]
		cpuRatio    = state.Resources.CPUs.Reserved / float64(state.Resources.CPUs.Total)
		memoryRatio = float64(state.Resources.Memory.Reserved) / float64(state.Resources.Memory.Total)
	)

	return (cpuRatio + memoryRatio) / 2.0
}
