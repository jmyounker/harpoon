package main

import (
	"os"
)

// Container defines the platform-agnostic interface for managing a container.
type Container interface {
	Start() error

	Wait() ContainerExitStatus

	// Signal sends sig to the container's init process.
	Signal(sig os.Signal)

	Metrics() ContainerMetrics
}
