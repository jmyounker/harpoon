// +build !linux

package main

import (
	"fmt"
	"os"
)

type container struct{}

func newContainer(config, rootfs string, args []string) Container {
	return &container{}
}

func (*container) Start() error {
	return fmt.Errorf("platform does not support containers")
}

func (*container) Wait() ContainerExitStatus {
	return ContainerExitStatus{}
}

func (*container) Signal(os.Signal) {}

func (*container) Metrics() ContainerMetrics {
	return ContainerMetrics{}
}
