package agentrepr

import (
	"fmt"
	"sync"
	"testing"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestFakeClient(t *testing.T) {
	const (
		endpoint = "doesn't matter"
		id       = "foobar"
	)

	c := NewFakeClient(t, endpoint, false)

	eventc, stopper, err := c.Events()
	if err != nil {
		t.Fatal(err)
	}
	defer stopper.Stop()

	bufferedc := make(chan agent.StateEvent, 1)
	next := func() agent.ContainerStatus {
		return (<-bufferedc).Containers[id].ContainerStatus
	}

	go func() {
		for e := range eventc {
			bufferedc <- e
		}
	}()

	next() // catch initial snapshot

	c.Put(id, agent.ContainerConfig{})
	if want, have := agent.ContainerStatusCreated, next(); want != have {
		t.Errorf("want %q, have %q", want, have)
	}

	c.Start(id)
	if want, have := agent.ContainerStatusRunning, next(); want != have {
		t.Errorf("want %q, have %q", want, have)
	}

	c.Stop(id)
	if want, have := agent.ContainerStatusFinished, next(); want != have {
		t.Errorf("want %q, have %q", want, have)
	}

	c.Destroy(id)
	if want, have := agent.ContainerStatusDeleted, next(); want != have {
		t.Errorf("want %q, have %q", want, have)
	}
}

type fakeClient struct {
	sync.RWMutex
	t         *testing.T
	endpoint  string
	inc       chan agent.StateEvent
	out       map[chan<- agent.StateEvent]struct{}
	instances map[string]agent.ContainerStatus
	fail      bool
}

func NewFakeClient(t *testing.T, endpoint string, fail bool) *fakeClient {
	return &fakeClient{
		t:         t,
		endpoint:  endpoint,
		inc:       make(chan agent.StateEvent),
		out:       map[chan<- agent.StateEvent]struct{}{},
		instances: map[string]agent.ContainerStatus{},
		fail:      fail,
	}
}

func (c *fakeClient) Endpoint() string {
	return c.endpoint
}

func (c *fakeClient) Events() (<-chan agent.StateEvent, agent.Stopper, error) {
	c.Lock()
	defer c.Unlock()

	var (
		outc  = make(chan agent.StateEvent)
		stopc = make(chan struct{})
	)

	c.out[outc] = struct{}{} // subscribe

	// initial snapshot
	instances := map[string]agent.ContainerInstance{}
	for id, st := range c.instances {
		instances[id] = agent.ContainerInstance{ContainerStatus: st}
	}
	go func() { outc <- agent.StateEvent{Containers: instances} }()

	// catch stop, unsubscribe, and clean up
	go func() {
		<-stopc

		c.Lock()
		defer c.Unlock()

		delete(c.out, outc)
		close(outc)
	}()

	return outc, stopper(stopc), nil
}

func (c *fakeClient) Create(id string, _ agent.ContainerConfig) error {
	c.Lock()
	defer c.Unlock()

	if _, ok := c.instances[id]; ok {
		return agent.ErrContainerAlreadyExists
	}

	c.instances[id] = agent.ContainerStatusCreated

	c.broadcast(id)

	return nil
}

func (c *fakeClient) Put(id string, _ agent.ContainerConfig) error {
	c.Lock()
	defer c.Unlock()

	if status, ok := c.instances[id]; ok && status != agent.ContainerStatusCreated {
		return agent.ErrContainerAlreadyExists
	}

	c.instances[id] = agent.ContainerStatusCreated

	c.broadcast(id)

	c.instances[id] = agent.ContainerStatusRunning

	c.broadcast(id)

	return nil
}

func (c *fakeClient) Start(id string) error {
	c.Lock()
	defer c.Unlock()

	s, ok := c.instances[id]
	if !ok {
		return agent.ErrContainerNotExist
	}

	switch s {
	case agent.ContainerStatusCreated, agent.ContainerStatusFailed, agent.ContainerStatusFinished:
	case agent.ContainerStatusRunning:
		return agent.ErrContainerAlreadyRunning
	default:
		return fmt.Errorf("%q is %s; can't start", id, s)
	}

	c.instances[id] = agent.ContainerStatusRunning

	c.broadcast(id)

	return nil
}

func (c *fakeClient) Force(id string, s agent.ContainerStatus) {
	c.Lock()
	defer c.Unlock()

	c.instances[id] = s

	c.broadcast(id)
}

func (c *fakeClient) Get(id string) (agent.ContainerStatus, bool) {
	c.RLock()
	defer c.RUnlock()

	s, ok := c.instances[id]
	return s, ok
}

func (c *fakeClient) Stop(id string) error {
	c.Lock()
	defer c.Unlock()

	s, ok := c.instances[id]
	if !ok {
		return agent.ErrContainerNotExist
	}

	switch s {
	case agent.ContainerStatusRunning:
	default:
		return agent.ErrContainerAlreadyStopped
	}

	c.instances[id] = agent.ContainerStatusFinished

	c.broadcast(id)

	return nil
}

func (c *fakeClient) Destroy(id string) error {
	c.Lock()
	defer c.Unlock()

	s, ok := c.instances[id]
	if !ok {
		return agent.ErrContainerNotExist
	}

	switch s {
	case agent.ContainerStatusCreated, agent.ContainerStatusFailed, agent.ContainerStatusFinished:
	default:
		return fmt.Errorf("%q is %s; can't delete", id, s)
	}

	c.instances[id] = agent.ContainerStatusDeleted

	c.broadcast(id)

	delete(c.instances, id)

	return nil
}

func (c *fakeClient) broadcast(id string) {
	if _, ok := c.instances[id]; !ok {
		panic("bad state in fakeClient broadcast")
	}

	e := agent.StateEvent{
		Containers: map[string]agent.ContainerInstance{
			id: agent.ContainerInstance{ContainerStatus: c.instances[id]},
		},
	}

	for ch := range c.out {
		ch <- e

		if c.fail {
			ch <- e
		}
	}
}

type stopper chan struct{}

func (s stopper) Stop() { close(s) }
