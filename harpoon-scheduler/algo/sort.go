// Package algo implements scheduling algorithms.
package algo

import (
	"sort"
)

type leastUsed struct {
	e2c       map[string]int // endpoint to container's count
	endpoints []string
}

func (s leastUsed) sort(endpoints []string) {
	s.endpoints = endpoints
	sort.Sort(s)
}

func (s leastUsed) Less(i, j int) bool {
	var (
		iContainersCount = s.e2c[s.endpoints[i]]
		jContainersCount = s.e2c[s.endpoints[j]]
	)

	return iContainersCount < jContainersCount

}

func (s leastUsed) Len() int {
	return len(s.endpoints)
}

func (s leastUsed) Swap(i, j int) {
	s.endpoints[i], s.endpoints[j] = s.endpoints[j], s.endpoints[i]
}
