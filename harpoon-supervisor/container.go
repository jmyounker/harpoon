package main

import (
	"os"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

// Container defines the platform-agnostic interface for managing a container.
type Container interface {
	Start() error

	Wait() agent.ContainerExitStatus

	// Signal sends sig to the container's init process.
	Signal(sig os.Signal)

	Metrics() agent.ContainerMetrics
}
