package main

import "github.com/soundcloud/harpoon/harpoon-agent/lib"

type fakeContainer struct {
	agent.ContainerInstance

	logs *containerLog

	subscribers map[chan<- agent.ContainerInstance]struct{}

	actionRequestc chan actionRequest
	hbRequestc     chan heartbeatRequest
	subc           chan chan<- agent.ContainerInstance
	unsubc         chan chan<- agent.ContainerInstance
	quitc          chan struct{}
}

func newFakeContainer(id string) *fakeContainer {
	c := &fakeContainer{
		ContainerInstance: agent.ContainerInstance{
			ID:     id,
			Status: agent.ContainerStatusRunning,
		},
		logs:           NewContainerLog(10000),
		subscribers:    map[chan<- agent.ContainerInstance]struct{}{},
		actionRequestc: make(chan actionRequest),
		hbRequestc:     make(chan heartbeatRequest),
		subc:           make(chan chan<- agent.ContainerInstance),
		unsubc:         make(chan chan<- agent.ContainerInstance),
		quitc:          make(chan struct{}),
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

func (c *fakeContainer) Heartbeat(hb agent.Heartbeat) string {
	req := heartbeatRequest{
		heartbeat: hb,
		res:       make(chan string),
	}
	c.hbRequestc <- req
	return <-req.res
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
		case req := <-c.hbRequestc:
			req.res <- c.heartbeat(req.heartbeat)
		case ch := <-c.subc:
			c.subscribers[ch] = struct{}{}
		case ch := <-c.unsubc:
			delete(c.subscribers, ch)
		case <-c.quitc:
			c.logs.Exit()
			return
		}
	}
}

func (c *fakeContainer) create() error {
	return nil
}

func (c *fakeContainer) destroy() error {
	c.updateStatus(agent.ContainerStatusDeleted)

	for subc := range c.subscribers {
		close(subc)
	}

	c.subscribers = map[chan<- agent.ContainerInstance]struct{}{}
	close(c.quitc)

	return nil
}

func (c *fakeContainer) heartbeat(hb agent.Heartbeat) string {
	return "UP"
}

func (c *fakeContainer) start() error {
	c.updateStatus(agent.ContainerStatusRunning)
	return nil
}

func (c *fakeContainer) stop() error {
	return nil
}

func (c *fakeContainer) updateStatus(status agent.ContainerStatus) {
	c.ContainerInstance.Status = status

	for subc := range c.subscribers {
		subc <- c.ContainerInstance
	}
}
