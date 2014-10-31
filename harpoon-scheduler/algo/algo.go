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

	var states = map[string]agent.StateEvent{}
	for id, state := range have {
		states[id] = state
	}

	for _, task := range pending {
		if task.Schedule {
			s := states[task.Endpoint]
			s.Resources.CPUs.Reserved += task.ContainerConfig.CPUs
			s.Resources.Memory.Reserved += task.ContainerConfig.Memory
			states[task.Endpoint] = s
		}
	}

	for id, config := range want {
		// Find all candidates
		valid := filter(states, config)
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
		s := states[chosen]
		s.Resources.CPUs.Reserved += config.CPUs
		s.Resources.Memory.Reserved += config.Memory
		states[chosen] = s
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
		states   = map[string]agent.StateEvent{}
		strategy = sortStrategy{}
	)

	for id, state := range have {
		states[id] = state
	}

	for id, task := range pending {
		if task.Schedule {
			s := states[task.Endpoint]
			s.Resources.CPUs.Reserved += task.ContainerConfig.CPUs
			s.Resources.Memory.Reserved += task.ContainerConfig.Memory
			states[task.Endpoint] = s
			states[task.Endpoint].Containers[id] = agent.ContainerInstance{ContainerConfig: task.ContainerConfig}
		}
	}

	for id, config := range want {
		// Find all candidates
		valid := filter(states, config)
		if len(valid) <= 0 {
			failed[id] = config
			continue
		}

		strategy.sort(valid, states)

		// Select a candidate
		chosen := strategy.best()

		// Place the container
		target, ok := mapped[chosen]
		if !ok {
			target = map[string]agent.ContainerConfig{}
		}
		target[id] = config
		mapped[chosen] = target

		// Adjust the resources
		s := states[chosen]
		s.Resources.CPUs.Reserved += config.CPUs
		s.Resources.Memory.Reserved += config.Memory
		s.Containers[id] = agent.ContainerInstance{ContainerConfig: config}
		states[chosen] = s
	}

	return mapped, failed
}

func filter(have map[string]agent.StateEvent, c agent.ContainerConfig) []string {
	valid := make([]string, 0, len(have))

	for endpoint, s := range have {
		if !match(c, s.Resources) {
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
