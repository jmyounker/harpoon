package main

import (
	"os"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type fakeContainer struct {
	startc  chan error
	signalc chan os.Signal
	waitc   chan agent.ContainerExitStatus
	metrics uint64
}

func newFakeContainer() *fakeContainer {
	return &fakeContainer{
		startc:  make(chan error),
		signalc: make(chan os.Signal, 1),
		waitc:   make(chan agent.ContainerExitStatus),
	}
}

func (c *fakeContainer) Start() error {
	return <-c.startc
}

func (c *fakeContainer) Wait() agent.ContainerExitStatus {
	return <-c.waitc
}

func (c *fakeContainer) Metrics() agent.ContainerMetrics {
	return agent.ContainerMetrics{
		CPUTime:     c.metrics,
		MemoryUsage: c.metrics,
		MemoryLimit: c.metrics,
	}
}

func (c *fakeContainer) Signal(sig os.Signal) {
	c.signalc <- sig
}
