package reprproxy

import (
	"errors"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-scheduler/agentrepr"
)

type fakeRepr struct {
	myEndpoint string
	subs       map[chan<- map[string]agent.StateEvent]struct{}
	scheduled  map[string]agent.ContainerConfig
	stopped    bool
}

// NewFakeRepr may be swapped for NewAgentRepresentation in tests.
func NewFakeRepr(endpoint string) agentrepr.Representation {
	m := &fakeRepr{
		myEndpoint: endpoint,
		subs:       map[chan<- map[string]agent.StateEvent]struct{}{},
		scheduled:  map[string]agent.ContainerConfig{},
		stopped:    false,
	}

	return m
}

func (r *fakeRepr) Endpoint() string {
	return r.myEndpoint
}

func (r *fakeRepr) Subscribe(c chan<- map[string]agent.StateEvent) {
	r.subs[c] = struct{}{}

	go func() { c <- r.Snapshot() }()
}

func (r *fakeRepr) Unsubscribe(c chan<- map[string]agent.StateEvent) {
	delete(r.subs, c)
}

func (r *fakeRepr) Schedule(id string, config agent.ContainerConfig) error {
	r.scheduled[id] = config

	r.broadcast()

	return nil
}

func (r *fakeRepr) Unschedule(id string) error {
	if _, ok := r.scheduled[id]; !ok {
		return errors.New("not scheduled")
	}

	delete(r.scheduled, id)

	r.broadcast()

	return nil
}

func (r *fakeRepr) Snapshot() map[string]agent.StateEvent {
	m := map[string]agent.ContainerInstance{}

	for id, c := range r.scheduled {
		m[id] = agent.ContainerInstance{
			ContainerConfig: c,
			ContainerStatus: agent.ContainerStatusRunning,
		}
	}

	return map[string]agent.StateEvent{r.myEndpoint: agent.StateEvent{Containers: m}}
}

func (r *fakeRepr) broadcast() {
	for c := range r.subs {
		go func(c chan<- map[string]agent.StateEvent) { c <- r.Snapshot() }(c)
	}
}

func (r *fakeRepr) Quit() {
	r.stopped = true
}
