package main

import (
	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

// taskSpec represents a scheduled container. They're emitted by scheduling
// algorithms.
type taskSpec struct {
	Endpoint              string `json:"endpoint"`
	Job                   string `json:"job"`
	ContainerID           string `json:"container_id"`
	agent.ContainerConfig `json:"container_config"`
}
