package main

import (
	"fmt"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type fakeContainer struct {
	agent.ContainerInstance

	logs        *containerLog
	subscribers map[chan<- agent.ContainerInstance]struct{}
	createc     chan createRequest
	destroyc    chan destroyRequest
	startc      chan startRequest
	stopc       chan stopRequest
	subc        chan chan<- agent.ContainerInstance
	unsubc      chan chan<- agent.ContainerInstance
	quitc       chan chan struct{}
}

// Satisfaction guaranteed.
var _ container = &fakeContainer{}

func newFakeContainer(
	id string,
	_ string,
	_ volumes,
	config agent.ContainerConfig,
	_ bool,
	_ *portDB,
	_ string,
) container {
	c := &fakeContainer{
		ContainerInstance: agent.ContainerInstance{
			ID:              id,
			ContainerStatus: agent.ContainerStatusRunning,
			ContainerConfig: config,
		},
		logs:        newContainerLog(containerLogRingBufferSize),
		subscribers: map[chan<- agent.ContainerInstance]struct{}{},
		createc:     make(chan createRequest),
		destroyc:    make(chan destroyRequest),
		startc:      make(chan startRequest),
		stopc:       make(chan stopRequest),
		subc:        make(chan chan<- agent.ContainerInstance),
		unsubc:      make(chan chan<- agent.ContainerInstance),
		quitc:       make(chan chan struct{}),
	}

	go c.loop()

	return c
}

func (c *fakeContainer) Create(unregister func(), downloadTimeout time.Duration) error {
	req := createRequest{
		unregister:      unregister,
		downloadTimeout: downloadTimeout,
		resp:            make(chan error),
	}
	c.createc <- req
	return <-req.resp
}

func (c *fakeContainer) Destroy() error {
	req := destroyRequest{
		resp: make(chan error),
	}
	c.destroyc <- req
	return <-req.resp
}

func (c *fakeContainer) Logs() *containerLog {
	return c.logs
}

func (c *fakeContainer) Instance() agent.ContainerInstance {
	return c.ContainerInstance
}

func (c *fakeContainer) Start() error {
	req := startRequest{
		resp: make(chan error),
	}
	c.startc <- req
	return <-req.resp
}

func (c *fakeContainer) Stop() error {
	req := stopRequest{
		resp: make(chan error),
	}
	c.stopc <- req
	return <-req.resp
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
		case req := <-c.createc:
			req.resp <- c.create()
		case req := <-c.destroyc:
			req.resp <- c.destroy()
		case req := <-c.startc:
			req.resp <- c.start()
		case req := <-c.stopc:
			req.resp <- c.stop()
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
