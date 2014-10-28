// Package algo implements scheduling algorithms.
package algo

import (
	"sort"
)

// SortStrategy implements sorting of endpoints
type SortStrategy interface {
	Sort(endpoints []string)
}

// LeastUsedSort returns a SortStrategy ordering endpoints by containers count
func LeastUsedSort(endpoint2containersCount map[string]int) SortStrategy {
	return leastUsed{
		endpoint2containersCount: endpoint2containersCount,
	}
}

type leastUsed struct {
	endpoint2containersCount map[string]int
	endpoints                []string
}

func (s leastUsed) Sort(endpoints []string) {
	s.endpoints = endpoints
	sort.Sort(s)
}

func (s leastUsed) Less(i, j int) bool {
	var (
		iContainersCount = s.endpoint2containersCount[s.endpoints[i]]
		jContainersCount = s.endpoint2containersCount[s.endpoints[j]]
	)

	return iContainersCount < jContainersCount

}

func (s leastUsed) Len() int {
	return len(s.endpoints)
}

func (s leastUsed) Swap(i, j int) {
	s.endpoints[i], s.endpoints[j] = s.endpoints[j], s.endpoints[i]
}
