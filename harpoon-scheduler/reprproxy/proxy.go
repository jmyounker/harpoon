// Package reprproxy implements a proxy for (or an aggregation of) multiple
// remote agent representations. The proxy is a single point of interaction
// for all agents.
package reprproxy

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-scheduler/agentrepr"
)

func newRealRepr(endpoint string) agentrepr.Representation {
	return agentrepr.New(agent.MustNewClient(endpoint))
}

var (
	// Debugf may be set from a controlling package.
	Debugf = func(string, ...interface{}) {}

	// NewAgentRepresentation is a factory function for creating
	// representations when the AgentDiscovery detects changes. It may be
	// swapped for tests.
	NewAgentRepresentation = newRealRepr

	initializeTimeout = 1 * time.Second // likewise
)

// Proxy defines everything needed from a Proxy implementation. It collects a
// bunch of individual remote agents, and provides a unified interface to
// them.
//
// Proxy safisfies the xf.ActualBroadcaster and xf.TaskScheduler interfaces.
type Proxy interface {
	Subscribe(c chan<- map[string]agent.StateEvent)
	Unsubscribe(c chan<- map[string]agent.StateEvent)
	Schedule(endpoint, id string, c agent.ContainerConfig) error
	Unschedule(endpoint, id string) error
	Snapshot() map[string]agent.StateEvent
	Quit()
}

type proxy struct {
	subc      chan chan<- map[string]agent.StateEvent
	unsubc    chan chan<- map[string]agent.StateEvent
	snapshotc chan map[string]agent.StateEvent
	quitc     chan struct{}

	*subscribers
	*agents
	*state

	sync.WaitGroup
}

// New creates and returns a new Aggreegator. It instantiates (and terminates)
// agent representations based on wisdom received from the AgentDiscovery
// component.
func New(d AgentDiscovery) *proxy {
	p := &proxy{
		subc:      make(chan chan<- map[string]agent.StateEvent),
		unsubc:    make(chan chan<- map[string]agent.StateEvent),
		snapshotc: make(chan map[string]agent.StateEvent),
		quitc:     make(chan struct{}),

		subscribers: newSubscribers(),
		agents:      newAgents(),
		state:       newState(),
	}

	p.WaitGroup.Add(1)
	go p.loop(d)

	return p
}

// Subscribe implements the xf.ActualBroadcaster interface.
func (p *proxy) Subscribe(c chan<- map[string]agent.StateEvent) {
	p.subc <- c
}

// Unsubscribe implements the xf.ActualBroadcaster interface.
func (p *proxy) Unsubscribe(c chan<- map[string]agent.StateEvent) {
	p.unsubc <- c
}

// Schedule implements the xf.TaskScheduler interface.
func (p *proxy) Schedule(endpoint, id string, c agent.ContainerConfig) error {
	agent, ok := p.agents.get(endpoint)
	if !ok {
		return fmt.Errorf("no agent found for %s", endpoint)
	}

	return agent.Schedule(id, c)
}

// Unschedule implements the xf.TaskScheduler interface.
func (p *proxy) Unschedule(endpoint, id string) error {
	agent, ok := p.agents.get(endpoint)
	if !ok {
		return fmt.Errorf("no agent found for %s", endpoint)
	}

	return agent.Unschedule(id)
}

// Snapshot returns the current state of the scheduling domain. It's not part
// of any interface; it's just used by the API.
func (p *proxy) Snapshot() map[string]agent.StateEvent {
	return <-p.snapshotc
}

// Quit terminates the proxy.
func (p *proxy) Quit() {
	close(p.quitc)
	p.WaitGroup.Wait()
}

func (p *proxy) loop(d AgentDiscovery) {
	defer p.WaitGroup.Done()

	var (
		discoveryc = make(chan []string)
		updatec    = make(chan map[string]agent.StateEvent)
	)

	d.Subscribe(discoveryc)
	defer d.Unsubscribe(discoveryc)

	p.initialize(discoveryc, updatec)

	// This request loop follows the same structure as the agentrepr. Namely:
	// any logic in a case block is moved to a method with the same name as
	// the originating channel.

	for {
		select {
		case endpoints := <-discoveryc:
			p.discovery(endpoints, updatec)

		case m := <-updatec:
			p.update(m)

		case c := <-p.subc:
			p.sub(c)

		case c := <-p.unsubc:
			p.unsub(c)

		case p.snapshotc <- p.snapshot():

		case <-p.quitc:
			return
		}
	}
}

func (p *proxy) initialize(discoveryc <-chan []string, updatec chan map[string]agent.StateEvent) {
	// Get the initial set of endpoints from discoveryc.
	var endpoints []string
	select {
	case endpoints = <-discoveryc:
	case <-time.After(time.Millisecond):
		panic("misbehaving agent discovery")
	}

	// Each of those endpoints will get an initial state dump.
	outstanding := map[string]struct{}{}
	for _, endpoint := range endpoints {
		outstanding[endpoint] = struct{}{}
	}

	// Add those endpoints to our agents structure.
	p.agents.update(endpoints, updatec)

	// We'll now get N initial state dumps on updatec.
	timeout := time.After(initializeTimeout)
	for len(outstanding) > 0 {
		select {
		case update := <-updatec:
			p.state.update(update)

			for endpoint := range update {
				delete(outstanding, endpoint)
			}

		case <-timeout:
			panic(fmt.Sprintf("timeout waiting for remote agent initialization; %d agent(s) remaining", len(outstanding)))
		}
	}
}

func (p *proxy) discovery(endpoints []string, updatec chan map[string]agent.StateEvent) {
	p.agents.update(endpoints, updatec)
	p.state.synchronize(p.agents.endpoints()) // potentially, purge lost agents
}

func (p *proxy) update(m map[string]agent.StateEvent) {
	p.state.update(m)
	p.subscribers.broadcast(p.state.copy())
}

func (p *proxy) sub(c chan<- map[string]agent.StateEvent) {
	p.subscribers.add(c)
	go func() { c <- p.snapshot() }()
}

func (p *proxy) unsub(c chan<- map[string]agent.StateEvent) {
	p.subscribers.remove(c)
}

func (p *proxy) snapshot() map[string]agent.StateEvent {
	return p.state.copy()
}

type subscribers struct {
	sync.RWMutex
	m map[chan<- map[string]agent.StateEvent]struct{}
}

func newSubscribers() *subscribers {
	return &subscribers{
		m: map[chan<- map[string]agent.StateEvent]struct{}{},
	}
}

func (s *subscribers) add(c chan<- map[string]agent.StateEvent) {
	s.Lock()
	defer s.Unlock()
	s.m[c] = struct{}{}
}

func (s *subscribers) remove(c chan<- map[string]agent.StateEvent) {
	s.Lock()
	defer s.Unlock()
	delete(s.m, c)
	close(c)
}

func (s *subscribers) broadcast(m map[string]agent.StateEvent) {
	s.RLock()
	defer s.RUnlock()

	for c := range s.m {
		select {
		case c <- m:
		case <-time.After(time.Millisecond):
			panic("slow representation subscriber")
		}
	}
}

func (s *subscribers) count() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.m)
}

type agents struct {
	sync.RWMutex
	m map[string]agentrepr.Representation
}

func newAgents() *agents {
	return &agents{
		m: map[string]agentrepr.Representation{},
	}
}

func (a *agents) update(endpoints []string, updatec chan<- map[string]agent.StateEvent) {
	a.Lock()
	defer a.Unlock()

	nextGen := map[string]agentrepr.Representation{}

	for _, endpoint := range endpoints {
		r, ok := a.m[endpoint]
		if !ok {
			r = NewAgentRepresentation(endpoint)
			r.Subscribe(updatec)
		}

		nextGen[endpoint] = r

		delete(a.m, endpoint)
	}

	// Leftovers
	for endpoint, r := range a.m {
		log.Printf("%s lost", endpoint)
		r.Unsubscribe(updatec)
		r.Quit()
	}

	a.m = nextGen
}

func (a *agents) get(endpoint string) (agentrepr.Representation, bool) {
	a.RLock()
	defer a.RUnlock()

	r, ok := a.m[endpoint]
	return r, ok
}

func (a *agents) endpoints() []string {
	a.RLock()
	defer a.RUnlock()

	endpoints := make([]string, 0, len(a.m))

	for endpoint := range a.m {
		endpoints = append(endpoints, endpoint)
	}

	return endpoints
}

type state struct {
	sync.RWMutex
	m map[string]agent.StateEvent
}

func newState() *state {
	return &state{
		m: map[string]agent.StateEvent{},
	}
}

func (s *state) update(m map[string]agent.StateEvent) {
	s.Lock()
	defer s.Unlock()

	for endpoint, se := range m {
		s.m[endpoint] = se
	}
}

func (s *state) copy() map[string]agent.StateEvent {
	s.RLock()
	defer s.RUnlock()

	m := make(map[string]agent.StateEvent, len(s.m))

	for k, v := range s.m {
		m[k] = v
	}

	return m
}

func (s *state) synchronize(activeEndpoints []string) {
	s.Lock()
	defer s.Unlock()

	active := make(map[string]struct{}, len(activeEndpoints))
	for _, s := range activeEndpoints {
		active[s] = struct{}{}
	}

	for endpoint, se := range s.m {
		if _, ok := active[endpoint]; !ok {
			log.Printf("%s lost; %d container(s) lost", endpoint, len(se.Containers))
			delete(s.m, endpoint)
		}
	}
}
