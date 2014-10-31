package algo

import (
	"math/rand"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

// PendingTask represents a task that has already been un/scheduled but it's still pending
// Located here in order to avoid circular dependency with "xf" package.
type PendingTask struct {
	Schedule bool // true = pending schedule; false = pending unschedule
	Deadline time.Time
	Endpoint string
	agent.ContainerConfig
}

// RandomChoice implements a demo scheduling algorithm. It's intended to be a
// demo, and it's not suitable for actual use.
func RandomChoice(
	want map[string]agent.ContainerConfig,
	have map[string]agent.StateEvent,
	pending map[string]PendingTask,
) (
	mapped map[string]map[string]agent.ContainerConfig,
	failed map[string]agent.ContainerConfig,
) {
	if len(want) <= 0 {
		return mapped, failed
	}

	var (
		endpoints = make([]string, 0, len(have))
	)

	for endpoint := range have {
		endpoints = append(endpoints, endpoint)
	}

	for id, config := range want {
		endpoint := endpoints[rand.Intn(len(endpoints))]

		placed, ok := mapped[endpoint]
		if !ok {
			placed = map[string]agent.ContainerConfig{}
		}

		placed[id] = config

		mapped[endpoint] = placed
	}

	return mapped, failed
}

// RandomFit implements a minimum viable scheduling algorithm. Containers are
// placed on a random agent that meets their constraints.
func RandomFit(
	want map[string]agent.ContainerConfig,
	have map[string]agent.StateEvent,
	pending map[string]PendingTask,
) (
	mapped map[string]map[string]agent.ContainerConfig,
	failed map[string]agent.ContainerConfig,
) {
	mapped = map[string]map[string]agent.ContainerConfig{}
	failed = map[string]agent.ContainerConfig{}

	var resources = map[string]agent.HostResources{}
	for id, state := range have {
		resources[id] = state.Resources
	}

	for _, task := range pending {
		if task.Schedule {
			r := resources[task.Endpoint]
			r.CPUs.Reserved += task.ContainerConfig.CPUs
			r.Memory.Reserved += task.ContainerConfig.Memory
			resources[task.Endpoint] = r
		}
	}

	for id, config := range want {
		// Find all candidates
		valid := filter(resources, config)
		if len(valid) <= 0 {
			failed[id] = config
			continue
		}

		// Select a candidate
		chosen := valid[rand.Intn(len(valid))]

		// Place the container
		target, ok := mapped[chosen]
		if !ok {
			target = map[string]agent.ContainerConfig{}
		}
		target[id] = config
		mapped[chosen] = target

		// Adjust the resources
		r := resources[chosen]
		r.CPUs.Reserved += config.CPUs
		r.Memory.Reserved += config.Memory
		resources[chosen] = r
	}

	return mapped, failed
}

// LeastUsed implements a minimum viable scheduling algorithm. Containers are
// placed on a agent that meets their constraints and runs least number of containers.
func LeastUsed(
	want map[string]agent.ContainerConfig,
	have map[string]agent.StateEvent,
	pending map[string]PendingTask,
) (
	mapped map[string]map[string]agent.ContainerConfig,
	failed map[string]agent.ContainerConfig,
) {
	mapped = map[string]map[string]agent.ContainerConfig{}
	failed = map[string]agent.ContainerConfig{}

	var (
		resources = map[string]agent.HostResources{}
		e2c       = map[string]int{} // endpoint to container's count
		strategy  = leastUsed{e2c: e2c}
	)

	for id, state := range have {
		resources[id] = state.Resources
		e2c[id] = len(state.Containers)
	}

	for _, task := range pending {
		if task.Schedule {
			r := resources[task.Endpoint]
			r.CPUs.Reserved += task.ContainerConfig.CPUs
			r.Memory.Reserved += task.ContainerConfig.Memory
			resources[task.Endpoint] = r
		}
		e2c[task.Endpoint]++
	}

	for id, config := range want {
		// Find all candidates
		valid := filter(resources, config)
		if len(valid) <= 0 {
			failed[id] = config
			continue
		}

		strategy.sort(valid)

		// Select a least used candidate
		chosen := valid[0]

		// Place the container
		target, ok := mapped[chosen]
		if !ok {
			target = map[string]agent.ContainerConfig{}
		}
		target[id] = config
		mapped[chosen] = target

		// Adjust the resources
		r := resources[chosen]
		r.CPUs.Reserved += config.CPUs
		r.Memory.Reserved += config.Memory
		resources[chosen] = r

		e2c[chosen]++
	}

	return mapped, failed
}

func filter(have map[string]agent.HostResources, c agent.ContainerConfig) []string {
	valid := make([]string, 0, len(have))

	for endpoint, r := range have {
		if !match(c, r) {
			continue
		}

		valid = append(valid, endpoint)
	}

	return valid
}

func match(c agent.ContainerConfig, r agent.HostResources) bool {
	if want, have := c.CPUs, r.CPUs.Total-r.CPUs.Reserved; want > have {
		return false
	}

	if want, have := c.Memory, r.Memory.Total-r.Memory.Reserved; want > have {
		return false
	}

	m := map[string]struct{}{}
	for _, v := range r.Volumes {
		m[v] = struct{}{}
	}

	for _, v := range c.Volumes {
		if _, ok := m[v]; !ok {
			return false
		}
	}

	return true
}
