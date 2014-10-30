// Package agentrepr implements a representation of a remote agent within the
// scheduler. It's kept in-sync via the event strear.
package agentrepr

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-scheduler/metrics"
	"github.com/soundcloud/harpoon/harpoon-scheduler/xtime"
)

var (
	// Debugf may be set from a controlling package.
	Debugf = func(string, ...interface{}) {}

	// ReconnectInterval is how long the state machine will wait after a
	// connection error (of any type) before attempting to reconnect.
	ReconnectInterval = 1 * time.Second

	// AbandonTimeout is how long a state machine will wait for a
	// nonresponsive remote agent to come back online. Once this duration is
	// elapsed, the state machine will consider all container instances lost.
	// Those containers will be re-scheduled on other agents.
	AbandonTimeout = 20 * time.Second //5 * time.Minute

	// PendingOperationTimeout dictates how long a schedule or unschedule
	// command may stay pending and unrealized before we give up and allow the
	// client to repeat the command.
	PendingOperationTimeout = 10 * time.Second //1 * time.Minute

	errAgentConnectionInterrupted = errors.New("agent connection interrupted")
	errTransactionPending         = errors.New("transaction already pending for this container")
)

// Representation captures all of the behavior required of a remote agent in
// the scheduler. It's an interface because we want to swap it out with
// programmable mocks in unit tests. The aggregator is the only consumer of
// this interface.
//
// Technically, Representation implements xf.ActualBroadcaster and a reduced
// form of xf.TaskScheduler. But to avoid the import loop, we don't make that
// explicit.
//
// Note that we still return a single-element map[string]agent.StateEvent in
// our broadcasts, because that way it's a lot easier for the receiving client
// to multiplex responses from Representations together.
type Representation interface {
	Endpoint() string
	Subscribe(chan<- map[string]agent.StateEvent)
	Unsubscribe(chan<- map[string]agent.StateEvent)
	Schedule(id string, c agent.ContainerConfig) error
	Unschedule(id string) error
	Snapshot() map[string]agent.StateEvent // tests only
	Quit()
}

type representation struct {
	subc          chan chan<- map[string]agent.StateEvent
	unsubc        chan chan<- map[string]agent.StateEvent
	connectedc    chan bool
	initializec   chan agent.StateEvent
	updatec       chan agent.StateEvent
	snapshotc     chan map[string]agent.StateEvent
	interruptionc chan struct{}
	successc      chan string
	failurec      chan string
	quitc         chan struct{}

	*instances
	*resources
	*subscribers
	*outstanding
	*connection

	Client

	sync.WaitGroup
}

// Client is a subset of agent.Agent methods that we use in the
// representation. Client may be satisfied by an agent.NewClient.
type Client interface {
	Endpoint() string
	Events() (<-chan agent.StateEvent, agent.Stopper, error)
	Put(string, agent.ContainerConfig) error
	Start(string) error
	Stop(string) error
	Delete(string) error
}

// New returns a new representation.
func New(c Client) *representation {
	r := &representation{
		subc:          make(chan chan<- map[string]agent.StateEvent),
		unsubc:        make(chan chan<- map[string]agent.StateEvent),
		connectedc:    make(chan bool),
		initializec:   make(chan agent.StateEvent),
		updatec:       make(chan agent.StateEvent),
		snapshotc:     make(chan map[string]agent.StateEvent),
		interruptionc: make(chan struct{}),
		successc:      make(chan string),
		failurec:      make(chan string),
		quitc:         make(chan struct{}),

		instances:   newInstances(),
		resources:   newResources(),
		subscribers: newSubscribers(),
		outstanding: newOutstanding(),
		connection:  newConnection(),

		Client: c,
	}

	r.WaitGroup.Add(2)
	go r.connectionLoop()
	go r.requestLoop()

	return r

}

// Endpoint implements the Representation interface.
func (r *representation) Endpoint() string {
	return r.Client.Endpoint()
}

// Connected implements the Representation interface.
func (r *representation) Connected() bool {
	return <-r.connectedc
}

// Subscribe implements the Representation interface.
func (r *representation) Subscribe(c chan<- map[string]agent.StateEvent) {
	r.subc <- c
}

// Unsubscribe implements the Representation interface.
func (r *representation) Unsubscribe(c chan<- map[string]agent.StateEvent) {
	r.unsubc <- c
}

// Schedule implements the Representation interface. Schedule is processed
// asynchronously, out of the main requestLoop. See sched for details.
func (r *representation) Schedule(id string, c agent.ContainerConfig) error {
	if !r.connection.connected() {
		return errAgentConnectionInterrupted
	}

	if r.outstanding.contains(id) {
		return errTransactionPending
	}

	switch err := r.Client.Put(id, c); err {
	case nil, agent.ErrContainerAlreadyExists:
	default:
		return err
	}

	// sched is a command, and commands are processed asynchronously, i.e. out
	// of the primary requestLoop. Therefore, when we signal Start to the
	// remote agent, the response event may get to the requestLoop before we
	// actually return from the method! So, we prepare our "outstanding" ahead
	// of time, and cancel it if the Start fails.

	r.outstanding.want(id, agent.ContainerStatusRunning, r.successc, r.failurec)

	r.instances.advanceOne(id, schedule) // it's OK if running signal gets there first

	r.broadcast()

	metrics.IncTransactionsCreated(1)

	return nil
}

// Unschedule implements the Representation interface.
func (r *representation) Unschedule(id string) error {
	if !r.connection.connected() {
		return errAgentConnectionInterrupted
	}

	if r.outstanding.contains(id) {
		return errTransactionPending
	}

	switch err := r.Client.Stop(id); err {
	case nil, agent.ErrContainerAlreadyStopped:
	default:
		return err
	}

	// (see sched, above)

	r.outstanding.want(id, agent.ContainerStatusDeleted, r.successc, r.failurec) // meta-status

	switch err := r.Client.Delete(id); err {
	case nil:
	default:
		r.outstanding.remove(id) // won't happen
		return err
	}

	r.instances.advanceOne(id, unschedule) // it's OK if delete signal gets there first

	r.broadcast()

	metrics.IncTransactionsCreated(1)

	return nil
}

// Snapshot provides the current state of the agent.
func (r *representation) Snapshot() map[string]agent.StateEvent {
	return <-r.snapshotc
}

// Quit implements the Representation interface.
func (r *representation) Quit() {
	close(r.quitc)
	r.WaitGroup.Wait()
}

func (r *representation) connectionLoop() {
	defer r.WaitGroup.Done()

	for {
		statec, stopper, err := r.Client.Events()
		if err != nil {
			log.Printf("%s: %s", r.Endpoint(), err)

			select {
			case <-r.quitc:
				return
			case <-xtime.After(ReconnectInterval):
				continue
			}
		}

		log.Printf("%s: connection established", r.Endpoint())
		metrics.IncAgentConnectionsEstablished(1)

		if err := r.readLoop(statec, stopper); err != nil {
			log.Printf("%s: %s", r.Endpoint(), err)
			metrics.IncAgentConnectionsInterrupted(1)

			r.interruptionc <- struct{}{} // signal

			select {
			case <-r.quitc:
				return
			case <-xtime.After(ReconnectInterval):
				continue
			}
		}

		return
	}
}

func (r *representation) readLoop(statec <-chan agent.StateEvent, stopper agent.Stopper) error {
	defer stopper.Stop()

	// When this function exits, we've lost our connection to the agent. When
	// that happens, in the request loop, we start an "abandon" timer. When
	// that timer fires, it flushes all container instance state, and thereby
	// signals all containers as lost. The abandon timer is reset when we send
	// the first successful state update over initializec.

	first := true

	for {
		select {
		case <-r.quitc:
			return nil

		case state, ok := <-statec:
			if !ok {
				// When we detect a connection error, we'll trigger the lost
				// timer, but otherwise remain optimistic. We make no
				// immediate change to our set of instances, and enter our
				// reconnect loop, trying to get back in a good state.
				return errAgentConnectionInterrupted
			}

			metrics.IncContainerEventsReceived(1)

			if first {
				r.initializec <- state // clears previous abandon timer
				first = false
				continue
			}

			r.updatec <- state
		}
	}
}

func (r *representation) requestLoop() {
	defer r.WaitGroup.Done()

	var (
		abandonc <-chan time.Time // initially nil
	)

	// Any logic in a case block should be moved to a method with the same
	// name as the originating channel. Exceptions for logging, and
	// manipulating the function local channels, above.

	for {
		select {
		case c := <-r.subc:
			Debugf("%s: subc", r.Endpoint())
			r.sub(c)
			Debugf("%s: %d subscriber(s)", r.Endpoint(), r.subscribers.count())

		case c := <-r.unsubc:
			Debugf("%s: unsubc", r.Endpoint())
			r.unsub(c)
			Debugf("%s: %d subscriber(s)", r.Endpoint(), r.subscribers.count())

		case r.connectedc <- r.connection.connected():
			Debugf("%s: connectedc", r.Endpoint())

		case state := <-r.initializec:
			Debugf("%s: initializec", r.Endpoint())
			abandonc = nil
			r.initialize(state)

		case state := <-r.updatec:
			//Debugf("%s: updatec", r.Endpoint())
			r.update(state)

		case r.snapshotc <- r.snapshot():
			Debugf("%s: snapshotc", r.Endpoint())

		case <-r.interruptionc:
			Debugf("%s: interruptionc", r.Endpoint())
			if abandonc == nil {
				abandonc = xtime.After(AbandonTimeout)
			}
			r.interruption()

		case id := <-r.successc:
			Debugf("%s: successc", r.Endpoint())
			r.success(id)

		case id := <-r.failurec:
			Debugf("%s: failurec", r.Endpoint())
			r.failure(id)

		case <-abandonc:
			Debugf("%s: abandonc", r.Endpoint())
			r.abandon()

		case <-r.quitc:
			Debugf("%s: quitc", r.Endpoint())
			return
		}
	}
}

func (r *representation) sub(c chan<- map[string]agent.StateEvent) {
	r.subscribers.add(c)
	go func() { c <- r.snapshot() }()
}

func (r *representation) unsub(c chan<- map[string]agent.StateEvent) {
	r.subscribers.remove(c)
}

func (r *representation) initialize(e agent.StateEvent) {
	r.connection.restored()
	r.update(e)
}

func (r *representation) update(e agent.StateEvent) {
	r.resources.set(e.Resources)
	r.instances.advanceMany(e.Containers)
	r.outstanding.signal(e.Containers)
	r.broadcast()
}

func (r *representation) snapshot() map[string]agent.StateEvent {
	return map[string]agent.StateEvent{
		r.Endpoint(): agent.StateEvent{
			Resources:  r.resources.copy(),
			Containers: r.instances.copy(),
		},
	}
}

func (r *representation) interruption() {
	r.connection.broken()
}

func (r *representation) success(id string) {
	r.outstanding.remove(id)

	// We don't advance or broadcast here, because the signal has already come
	// on the event stream, and we've already caught it and done the work. We
	// just need to do our bookkeeping.

	metrics.IncTransactionsResolved(1)
}

func (r *representation) failure(id string) {
	r.outstanding.remove(id)

	r.instances.advanceOne(id, timeout)

	r.broadcast()

	metrics.IncTransactionsFailed(1)
}

func (r *representation) abandon() {
	r.resources.reset()

	r.instances.reset()

	r.outstanding.reset()

	r.broadcast()
}

func (r *representation) broadcast() {
	r.subscribers.broadcast(r.snapshot())
}

type instances struct {
	sync.RWMutex
	m map[string]stateInstance
}

func newInstances() *instances {
	return &instances{
		m: map[string]stateInstance{},
	}
}

func (i *instances) advanceOne(id string, t transition) {
	i.Lock()
	defer i.Unlock()

	var ci agent.ContainerInstance
	if si, ok := i.m[id]; ok {
		ci = si.ContainerInstance
	}

	transitions := map[string]transitionInstance{
		id: transitionInstance{t, ci},
	}

	i.m = step(transitions)(i.m)
}

func (i *instances) advanceMany(m map[string]agent.ContainerInstance) {
	i.Lock()
	defer i.Unlock()

	transitions := map[string]transitionInstance{}
	for id, ci := range m {
		transitions[id] = transitionInstance{s2t(ci.ContainerStatus), ci}
	}

	i.m = step(transitions)(i.m)
}

func (i *instances) copy() map[string]agent.ContainerInstance {
	i.RLock()
	defer i.RUnlock()

	m := map[string]agent.ContainerInstance{}

	for id, si := range i.m {
		m[id] = si.ContainerInstance
	}

	return m
}

func (i *instances) reset() {
	i.Lock()
	defer i.Unlock()
	i.m = map[string]stateInstance{}
}

type resources struct {
	sync.RWMutex
	agent.HostResources
}

func newResources() *resources {
	return &resources{}
}

func (r *resources) set(hr agent.HostResources) {
	r.Lock()
	defer r.Unlock()
	r.HostResources = hr
}

func (r *resources) copy() agent.HostResources {
	r.RLock()
	defer r.RUnlock()
	return r.HostResources
}

func (r *resources) reset() {
	r.Lock()
	defer r.Unlock()
	r.HostResources = agent.HostResources{}
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

type outstanding struct {
	sync.RWMutex
	m map[string]chan<- agent.ContainerStatus
}

func newOutstanding() *outstanding {
	return &outstanding{
		m: map[string]chan<- agent.ContainerStatus{},
	}
}

func (o *outstanding) contains(id string) bool {
	o.RLock()
	defer o.RUnlock()

	_, ok := o.m[id]
	return ok
}

func (o *outstanding) want(id string, s agent.ContainerStatus, successc, failurec chan string) {
	var (
		c = make(chan agent.ContainerStatus)
		t = xtime.After(PendingOperationTimeout)
	)

	go func() {
		received := false

		for {
			select {
			case status, ok := <-c:
				if !ok {
					if !received {
						failurec <- id
					}

					return
				}

				if status == s {
					received = true
					go func() { successc <- id }()
				}
			case <-t:
				if received {
					continue
				}

				received = true
				go func() { failurec <- id }()
			}

		}
	}()

	o.Lock()
	defer o.Unlock()
	o.m[id] = c
}

func (o *outstanding) signal(m map[string]agent.ContainerInstance) {
	o.RLock()
	defer o.RUnlock()

	for id, ci := range m {
		if c, ok := o.m[id]; ok {
			c <- ci.ContainerStatus
		}
	}
}

func (o *outstanding) remove(id string) {
	o.Lock()
	defer o.Unlock()
	delete(o.m, id)
}

func (o *outstanding) reset() {
	o.Lock()
	defer o.Unlock()

	for _, c := range o.m {
		close(c)
	}

	o.m = map[string]chan<- agent.ContainerStatus{}
}

type connection struct {
	sync.RWMutex
	good bool
}

func newConnection() *connection {
	return &connection{
		good: false,
	}
}

func (c *connection) connected() bool {
	c.RLock()
	defer c.RUnlock()
	return c.good
}

func (c *connection) broken() {
	c.Lock()
	defer c.Unlock()
	c.good = false
}

func (c *connection) restored() {
	c.Lock()
	defer c.Unlock()
	c.good = true
}

type stateInstance struct {
	stateFn
	agent.ContainerInstance
}

type transitionInstance struct {
	transition
	agent.ContainerInstance
}

type stepFn func(map[string]stateInstance) map[string]stateInstance

func step(transitions map[string]transitionInstance) stepFn {
	return func(instances map[string]stateInstance) map[string]stateInstance {
		for id, ti := range transitions {
			si, ok := instances[id]
			if !ok {
				si = stateInstance{stateFn: initialState}
			}

			Debugf("container event: %s (%s) %s", id, si.stateFn, ti.transition)

			// We blindly write the ContainerInstance from the incoming
			// transitionInstance. In some cases (scheduling a not-yet-
			// existent container) the ContainerInstance will be empty, and
			// that's OK, because we never introspect it.
			si.stateFn = si.stateFn(ti.transition)
			si.ContainerInstance = ti.ContainerInstance

			Debugf("container event: %s => %s", id, si.stateFn)

			if si.stateFn == nil {
				delete(instances, id)
				continue
			}

			instances[id] = si
		}

		return instances // convenience return only; parameter is mutated
	}
}
