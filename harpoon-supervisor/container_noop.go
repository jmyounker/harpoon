// +build !linux

package main

import (
	"fmt"
	"os"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type container struct{}

func newContainer(hostname string, id string, agentConfig, containerConfig, rootfs string, args []string) Container {
	return &container{}
}

func (*container) Start() error {
	return fmt.Errorf("platform does not support containers")
}

func (*container) Wait() agent.ContainerExitStatus {
	return agent.ContainerExitStatus{}
}

func (*container) Signal(os.Signal) {}

func (*container) Metrics() agent.ContainerMetrics {
	return agent.ContainerMetrics{}
}
