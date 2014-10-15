package main

import (
	"os"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type fakeContainer struct {
	startc  chan error
	signalc chan os.Signal
	waitc   chan agent.ContainerExitStatus
	restart agent.Restart
}

func newFakeContainer(restart agent.Restart) *fakeContainer {
	return &fakeContainer{
		startc:  make(chan error),
		signalc: make(chan os.Signal, 1),
		waitc:   make(chan agent.ContainerExitStatus),
		restart: restart,
	}
}

func (c *fakeContainer) Start() error {
	return <-c.startc
}

func (c *fakeContainer) Wait() agent.ContainerExitStatus {
	return <-c.waitc
}

func (c *fakeContainer) Metrics() agent.ContainerMetrics {
	return agent.ContainerMetrics{}
}

func (c *fakeContainer) Signal(sig os.Signal) {
	c.signalc <- sig
}

func (c *fakeContainer) Config() agent.ContainerConfig {
	return agent.ContainerConfig{Restart: c.restart}
}
