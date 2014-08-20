package main

import (
	"log"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type transformer struct {
	quitc chan chan struct{}
}

func newTransformer(actual actualBroadcaster, desired desiredBroadcaster, target taskScheduler) *transformer {
	t := &transformer{
		quitc: make(chan chan struct{}),
	}
	go t.loop(actual, desired, target)
	return t
}

func (t *transformer) quit() {
	q := make(chan struct{})
	t.quitc <- q
	<-q
}

func (t *transformer) loop(actual actualBroadcaster, desired desiredBroadcaster, target taskScheduler) {
	var (
		ticker   = time.NewTicker(5 * time.Second)
		actualc  = make(chan map[string]map[string]agent.ContainerInstance)
		desiredc = make(chan map[string]taskSpec)
		have     = map[string]map[string]agent.ContainerInstance{}
		want     = map[string]taskSpec{}
	)

	defer ticker.Stop()

	actual.subscribe(actualc)
	defer actual.unsubscribe(actualc)

	select {
	case have = <-actualc:
	case <-time.After(time.Millisecond):
		panic("misbehaving actual-state broadcaster")
	}

	desired.subscribe(desiredc)
	defer desired.unsubscribe(desiredc)

	select {
	case want = <-desiredc:
	case <-time.After(time.Millisecond):
		panic("misbehaving desired-state broadcaster")
	}

	for {
		select {
		case <-ticker.C:
			transform(want, have, target)

		case want = <-desiredc:
			transform(want, have, target)

		case have = <-actualc:
			transform(want, have, target)

		case q := <-t.quitc:
			close(q)
			return
		}
	}
}

func transform(want map[string]taskSpec, have map[string]map[string]agent.ContainerInstance, target taskScheduler) {
	// We need to make sure we don't double-schedule stuff. I think the best
	// way is to lean on the state in the actual agents. That is, any mutation
	// should first do a round-trip to the agent, to make sure the action
	// isn't already underway.

	todo := []func() error{}

	// Anything we want but don't have should be started.

	for id, spec := range want {
		instances, ok := have[spec.Endpoint]
		if !ok {
			// The desired endpoint has nothing scheduled.
			todo = append(todo, func() error { return target.schedule(spec) })
			continue
		}

		if _, ok := instances[id]; !ok {
			// The desired endpoint doesn't have this container scheduled.
			todo = append(todo, func() error { return target.schedule(spec) })
			continue
		}
	}

	// Anything we have but don't want should be stopped.

	for endpoint, instances := range have {
		for id := range instances {
			spec, ok := want[id]
			if !ok {
				// The existing container isn't in our desired set at all.
				todo = append(todo, func() error { return target.unschedule(endpoint, id) })
				continue
			}

			if endpoint != spec.Endpoint {
				// The existing container is in our desired set, but not on this endpoint.
				todo = append(todo, func() error { return target.unschedule(endpoint, id) })
				continue
			}
		}
	}

	// Engage.

	for _, fn := range todo {
		if err := fn(); err != nil {
			log.Printf("transformer: during transform: %s", err)
		}
	}
}
