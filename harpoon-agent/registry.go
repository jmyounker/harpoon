package main

import "github.com/soundcloud/harpoon/harpoon-agent/lib"

type registry struct {
	statec         chan agent.ContainerInstance
	removec        chan string
	notifyc        chan chan<- agent.ContainerInstance
	stopc          chan chan<- agent.ContainerInstance
	lenc           chan chan<- int
	instancesc     chan chan<- map[string]agent.ContainerInstance
	acceptUpdatesc chan bool
	getc           chan getRequest
	registerc      chan registerRequest
}

type getRequest struct {
	id   string
	outc chan container
}

type registerRequest struct {
	c    container
	outc chan bool
}

func newRegistry(sd serviceDiscovery) *registry {
	r := &registry{
		acceptUpdatesc: make(chan bool),
		statec:         make(chan agent.ContainerInstance),
		registerc:      make(chan registerRequest),
		removec:        make(chan string),
		notifyc:        make(chan chan<- agent.ContainerInstance),
		stopc:          make(chan chan<- agent.ContainerInstance),
		lenc:           make(chan chan<- int),
		instancesc:     make(chan chan<- map[string]agent.ContainerInstance),
		getc:           make(chan getRequest),
	}

	go r.loop(sd)

	return r
}

func (r *registry) remove(id string) {
	r.removec <- id
}

func (r *registry) get(id string) (container, bool) {
	outc := make(chan container)
	r.getc <- getRequest{id: id, outc: outc}
	c := <-outc
	if c == nil {
		return c, false
	}
	return c, true
}

func (r *registry) register(c container) bool {
	outc := make(chan bool)
	r.registerc <- registerRequest{c: c, outc: outc}
	return <-outc
}

func (r *registry) registerUnsafe(m map[string]container, c container) bool {
	if _, ok := m[c.Instance().ID]; ok {
		return false
	}

	m[c.Instance().ID] = c

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
	intc := make(chan int)
	r.lenc <- intc
	return <-intc
}

func (r *registry) instances() map[string]agent.ContainerInstance {
	i := make(chan map[string]agent.ContainerInstance)
	r.instancesc <- i
	return <-i
}

func (r *registry) acceptStateUpdates() {
	r.acceptUpdatesc <- true
}

func (r *registry) notify(c chan<- agent.ContainerInstance) {
	r.notifyc <- c
}

func (r *registry) stop(c chan<- agent.ContainerInstance) {
	r.stopc <- c
}

// Report state changes in any container to all of our subscribers.
func (r *registry) loop(sd serviceDiscovery) {
	var (
		m           = make(map[string]container)
		subscribers = make(map[chan<- agent.ContainerInstance]struct{})
		// acceptUpdates bool
	)
	// Report state changes in any container to all of our subscribers.
	for {
		select {
		case <-r.acceptUpdatesc:
			// acceptUpdates = b

		case containerInstance := <-r.statec:
			for subc := range subscribers {
				// Each channel send is being executed in a separate goroutine
				// because each subscriber can call back into this for-select
				// loop, causing a deadlock. If we move goroutine up one level
				// around the for-range loop on r.subscribers, we allow
				// concurrent access to the r.subscribers map.
				go func(s chan<- agent.ContainerInstance) {
					s <- containerInstance
				}(subc)
			}
			// TODO: we eventually want to include container state information
			// in what we send to service discovery, so we'll need to trigger
			// an Update here, too.

		case req := <-r.registerc:
			ok := r.registerUnsafe(m, req.c)
			if ok {
				sd.Update(instances(m))
			}
			req.outc <- ok

		case id := <-r.removec:
			delete(m, id)
			sd.Update(instances(m))

		case c := <-r.notifyc:
			subscribers[c] = struct{}{}

		case c := <-r.stopc:
			delete(subscribers, c)

		case outc := <-r.lenc:
			outc <- len(m)

		case outc := <-r.instancesc:
			outc <- instances(m)

		case req := <-r.getc:
			req.outc <- m[req.id]
		}
	}
}

func instances(m map[string]container) map[string]agent.ContainerInstance {
	instances := make(map[string]agent.ContainerInstance, len(m))
	for id, container := range m {
		instances[id] = container.Instance()
	}
	return instances
}
