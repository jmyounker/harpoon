package main

import (
	"sync"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type registry struct {
	m           map[string]container
	statec      chan agent.ContainerInstance
	subscribers map[chan<- agent.ContainerInstance]struct{}

	acceptUpdates bool

	sync.RWMutex
}

func newRegistry() *registry {
	r := &registry{
		m:           map[string]container{},
		statec:      make(chan agent.ContainerInstance),
		subscribers: map[chan<- agent.ContainerInstance]struct{}{},
	}

	go r.loop()

	return r
}

func (r *registry) remove(id string) {
	r.Lock()
	defer r.Unlock()

	delete(r.m, id)
}

func (r *registry) get(id string) (container, bool) {
	r.RLock()
	defer r.RUnlock()

	c, ok := r.m[id]
	return c, ok
}

func (r *registry) register(c container) bool {
	r.Lock()
	defer r.Unlock()

	if _, ok := r.m[c.Instance().ID]; ok {
		return false
	}

	r.m[c.Instance().ID] = c

	// The container sends us a copy of its associated ContainerInstance every
	// time the container changes state. This needs to happen outside of the
	// goroutine, to make sure we collect all initial state change(s).
	inc := make(chan agent.ContainerInstance)
	c.Subscribe(inc)

	// Forward the container's state changes to all subscribers.
	go func() {
		defer c.Unsubscribe(inc)
		for instance := range inc {
			r.statec <- instance
		}
	}()

	return true
}

func (r *registry) len() int {
	r.RLock()
	defer r.RUnlock()

	return len(r.m)
}

func (r *registry) instances() map[string]agent.ContainerInstance {
	r.Lock()
	defer r.Unlock()

	m := make(map[string]agent.ContainerInstance, len(r.m))

	for id, container := range r.m {
		m[id] = container.Instance()
	}

	return m
}

func (r *registry) acceptStateUpdates() {
	r.Lock()
	defer r.Unlock()

	r.acceptUpdates = true // TODO(pb): this isn't used anywhere
}

func (r *registry) notify(c chan<- agent.ContainerInstance) {
	r.Lock()
	defer r.Unlock()

	r.subscribers[c] = struct{}{}
}

func (r *registry) stop(c chan<- agent.ContainerInstance) {
	r.Lock()
	defer r.Unlock()

	delete(r.subscribers, c)
}

// Report state changes in any container to all of our subscribers.
func (r *registry) loop() {
	// Report state changes in any container to all of our subscribers.
	for containerInstance := range r.statec {
		r.RLock()
		s := make(map[chan<- agent.ContainerInstance]struct{}, len(r.subscribers))
		for k, v := range r.subscribers {
			s[k] = v
		}
		r.RUnlock()
		for subc := range s {
			subc <- containerInstance
		}
	}
}
