// Agent discovery allows components to find out about the remote agents
// available in a scheduling domain.
package main

import "sync"

type agentDiscovery interface {
	subscribe(chan<- []string)
	unsubscribe(chan<- []string)
}

type staticAgentDiscovery []string

func (d staticAgentDiscovery) subscribe(c chan<- []string) { go func() { c <- d }() }
func (d staticAgentDiscovery) unsubscribe(chan<- []string) { return }

type manualAgentDiscovery struct {
	sync.Mutex
	endpoints     []string // slice, to get deterministic order for tests
	subscriptions map[chan<- []string]struct{}
}

func newManualAgentDiscovery(endpoints []string) *manualAgentDiscovery {
	return &manualAgentDiscovery{
		endpoints:     endpoints,
		subscriptions: map[chan<- []string]struct{}{},
	}
}

func (d *manualAgentDiscovery) subscribe(c chan<- []string) {
	d.Lock()
	defer d.Unlock()

	d.subscriptions[c] = struct{}{}
	go func() { c <- d.endpoints }()
}

func (d *manualAgentDiscovery) unsubscribe(c chan<- []string) {
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
