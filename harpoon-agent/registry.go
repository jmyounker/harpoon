package main

import (
	"sync"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type registry struct {
	m            map[string]*container
	stateChanges chan agent.ContainerInstance
	subscribers  map[chan<- agent.ContainerInstance]struct{}

	acceptUpdates bool

	sync.RWMutex
}

func newRegistry() *registry {
	r := &registry{
		m:            map[string]*container{},
		stateChanges: make(chan agent.ContainerInstance),
		subscribers:  map[chan<- agent.ContainerInstance]struct{}{},
	}

	go r.loop()

	return r
}

func (r *registry) Remove(id string) {
	r.Lock()
	defer r.Unlock()

	delete(r.m, id)
}

func (r *registry) Get(id string) (*container, bool) {
	r.RLock()
	defer r.RUnlock()

	c, ok := r.m[id]
	return c, ok
}

func (r *registry) Register(c *container) bool {
	r.Lock()
	defer r.Unlock()

	if _, ok := r.m[c.ID]; ok {
		return false
	}

	r.m[c.ID] = c

	// Watch ContainerInstances for state changes and send these
	// to r.statec.  The loop() function picks up these changes
	// forwards them to the subscribers.
	go func(c *container, changesOut chan agent.ContainerInstance) {
		var (
			inc = make(chan agent.ContainerInstance)
		)
		// The container sends us a copy of its associated ContainerInstance every
		// time the container changes state.
		c.Subscribe(inc)
		defer c.Unsubscribe(inc)

		// Then we forward the modified ContainerInstances to r.stateChanges for reporting
		// to the registry's subscribers.
		for {
			select {
			// The channel is closed when the registered container is deleted, so we exit
			// the goroutine since there will be no more state changes.
			case instance, ok := <-inc:
				if !ok {
					return
				}
				changesOut <- instance
			}
		}
	}(c, r.stateChanges)

	return true
}

func (r *registry) Len() int {
	r.RLock()
	defer r.RUnlock()

	return len(r.m)
}

func (r *registry) Instances() []agent.ContainerInstance {
	r.Lock()
	defer r.Unlock()

	list := make([]agent.ContainerInstance, 0, len(r.m))

	for _, container := range r.m {
		list = append(list, container.Instance())
	}

	return list
}

func (r *registry) AcceptStateUpdates() {
	r.Lock()
	defer r.Unlock()

	r.acceptUpdates = true
}

func (r *registry) Notify(c chan<- agent.ContainerInstance) {
	r.Lock()
	defer r.Unlock()

	r.subscribers[c] = struct{}{}
}

func (r *registry) Stop(c chan<- agent.ContainerInstance) {
	r.Lock()
	defer r.Unlock()

	delete(r.subscribers, c)
}

// Report state changes in any container to all of our subscribers.
func (r *registry) loop() {
	// Report state changes in any container to all of our subscribers.
	for stateChange := range r.stateChanges {
		r.notifySubscribers(stateChange)
	}
}

func (r *registry) notifySubscribers(stateChange agent.ContainerInstance) {
	r.RLock()
	defer r.RUnlock()

	for subc := range r.subscribers {
		subc <- stateChange
	}
}
