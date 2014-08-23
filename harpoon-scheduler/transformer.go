package main

import (
	"log"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

var algo = randomChoice

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
		desiredc = make(chan map[string]configstore.JobConfig)
		have     = map[string]map[string]agent.ContainerInstance{}
		want     = map[string]configstore.JobConfig{}
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

func transform(wantJobs map[string]configstore.JobConfig, haveInstances map[string]map[string]agent.ContainerInstance, target taskScheduler) {

	var (
		todo        = []func() error{}
		wantTasks   = map[string]agent.ContainerConfig{}
		haveTasks   = map[string]agent.ContainerConfig{}
		id2endpoint = map[string]string{}
	)

	for _, cfg := range wantJobs {
		for i := 0; i < cfg.Scale; i++ {
			wantTasks[makeContainerID(cfg, i)] = cfg.ContainerConfig
		}
	}

	for endpoint, instances := range haveInstances {
		for id, instance := range instances {
			haveTasks[id] = instance.Config
			id2endpoint[id] = endpoint
		}
	}

	// Anything we want but don't have should be started.

	for id, cfg := range wantTasks {
		if _, ok := haveTasks[id]; ok {
			delete(wantTasks, id)
			delete(haveTasks, id)
			continue
		}

		endpoint, err := algo(cfg, haveInstances)
		if err != nil {
			log.Printf("transformer: error scheduling %s: %s", id, err)
			continue
		}

		todo = append(todo, func() error { return target.schedule(endpoint, id, cfg) })
	}

	// Anything we have but don't want should be stopped.

	for id := range haveTasks {
		endpoint, ok := id2endpoint[id]
		if !ok {
			panic("invalid state in transform")
		}

		todo = append(todo, func() error { return target.unschedule(endpoint, id) })
	}

	// Engage.

	for _, fn := range todo {
		if err := fn(); err != nil {
			log.Printf("transformer: during transform: %s", err)
		}
	}
}
