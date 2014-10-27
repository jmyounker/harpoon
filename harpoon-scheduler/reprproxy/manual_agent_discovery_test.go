package reprproxy_test

import "sync"

// manualAgentDiscovery encodes a set of endpoints which can be manually
// manipulated. TODO(pb): move to the _test package.
type manualAgentDiscovery struct {
	sync.Mutex
	endpoints     []string // slice, to get deterministic order for tests
	subscriptions map[chan<- []string]struct{}
}

func newManualAgentDiscovery(endpoints ...string) *manualAgentDiscovery {
	return &manualAgentDiscovery{
		endpoints:     endpoints,
		subscriptions: map[chan<- []string]struct{}{},
	}
}

// Subscribe satisfies the AgentDiscovery interface.
func (d *manualAgentDiscovery) Subscribe(c chan<- []string) {
	d.Lock()
	defer d.Unlock()

	d.subscriptions[c] = struct{}{}
	go func() { c <- d.endpoints }()
}

// Unsubscribe satisfies the AgentDiscovery interface.
func (d *manualAgentDiscovery) Unsubscribe(c chan<- []string) {
	d.Lock()
	defer d.Unlock()

	delete(d.subscriptions, c)
}

func (d *manualAgentDiscovery) add(endpoint string) {
	d.Lock()
	defer d.Unlock()

	for _, candidate := range d.endpoints {
		if endpoint == candidate {
			return
		}
	}

	d.endpoints = append(d.endpoints, endpoint)
	d.broadcast()
}

func (d *manualAgentDiscovery) del(endpoint string) {
	d.Lock()
	defer d.Unlock()

	var list = []string{}

	for _, candidate := range d.endpoints {
		if endpoint == candidate {
			continue
		}
		list = append(list, candidate)
	}

	d.endpoints = list
	d.broadcast()
}

func (d *manualAgentDiscovery) broadcast() {
	for c := range d.subscriptions {
		c <- d.endpoints
	}
}
