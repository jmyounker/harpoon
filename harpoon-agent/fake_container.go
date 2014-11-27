package main

import (
	"fmt"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type fakeContainer struct {
	agent.ContainerInstance

	logs *containerLog

	subscribers map[chan<- agent.ContainerInstance]struct{}

	actionRequestc chan actionRequest
	subc           chan chan<- agent.ContainerInstance
	unsubc         chan chan<- agent.ContainerInstance
	quitc          chan chan struct{}
}

// Satisfaction guaranteed.
var _ container = &fakeContainer{}

func newFakeContainer(
	id string,
	_ string,
	_ volumes,
	config agent.ContainerConfig,
	_ bool,
	_ *portDB) container {
	c := &fakeContainer{
		ContainerInstance: agent.ContainerInstance{
			ID:              id,
			ContainerStatus: agent.ContainerStatusRunning,
			ContainerConfig: config,
		},
		logs:           newContainerLog(containerLogRingBufferSize),
		subscribers:    map[chan<- agent.ContainerInstance]struct{}{},
		actionRequestc: make(chan actionRequest),
		subc:           make(chan chan<- agent.ContainerInstance),
		unsubc:         make(chan chan<- agent.ContainerInstance),
		quitc:          make(chan chan struct{}),
	}

	go c.loop()

	return c
}

func (c *fakeContainer) Create() error {
	req := actionRequest{
		action: containerCreate,
		res:    make(chan error),
	}
	c.actionRequestc <- req
	return <-req.res
}

func (c *fakeContainer) Destroy() error {
	req := actionRequest{
		action: containerDestroy,
		res:    make(chan error),
	}
	c.actionRequestc <- req
	return <-req.res
}

func (c *fakeContainer) Logs() *containerLog {
	return c.logs
}

func (c *fakeContainer) Instance() agent.ContainerInstance {
	return c.ContainerInstance
}

func (c *fakeContainer) Start() error {
	req := actionRequest{
		action: containerStart,
		res:    make(chan error),
	}
	c.actionRequestc <- req
	return <-req.res
}

func (c *fakeContainer) Stop() error {
	req := actionRequest{
		action: containerStop,
		res:    make(chan error),
	}
	c.actionRequestc <- req
	return <-req.res
}

func (c *fakeContainer) Recover() error {
	return nil
}

func (c *fakeContainer) Exit() {
	q := make(chan struct{})
	c.quitc <- q
	<-q
}

func (c *fakeContainer) Subscribe(ch chan<- agent.ContainerInstance) {
	c.subc <- ch
}

func (c *fakeContainer) Unsubscribe(ch chan<- agent.ContainerInstance) {
	c.unsubc <- ch
}

func (c *fakeContainer) loop() {
	for {
		select {
		case req := <-c.actionRequestc:
			switch req.action {
			case containerCreate:
				req.res <- c.create()
			case containerDestroy:
				req.res <- c.destroy()
			case containerStart:
				req.res <- c.start()
			case containerStop:
				req.res <- c.stop()
			default:
				panic("unknown action")
			}
		case ch := <-c.subc:
			c.subscribers[ch] = struct{}{}
		case ch := <-c.unsubc:
			delete(c.subscribers, ch)
		case q := <-c.quitc:
			c.logs.exit()
			close(q)
			return
		}
	}
}

const failingArtifactURL = "http://fail-forever.biz/no-container.tgz"

func (c *fakeContainer) create() error {
	if c.ContainerConfig.ArtifactURL == failingArtifactURL {
		return fmt.Errorf("failed to fetch")
	}
	return nil
}

func (c *fakeContainer) destroy() error {
	c.updateStatus(agent.ContainerStatusDeleted)
	for subc := range c.subscribers {
		close(subc)
	}

	c.subscribers = map[chan<- agent.ContainerInstance]struct{}{}

	return nil
}

func (c *fakeContainer) start() error {
	c.updateStatus(agent.ContainerStatusRunning)
	return nil
}

func (c *fakeContainer) stop() error {
	return nil
}

func (c *fakeContainer) updateStatus(status agent.ContainerStatus) {
	c.ContainerInstance.ContainerStatus = status

	for subc := range c.subscribers {
		subc <- c.ContainerInstance
	}
}
