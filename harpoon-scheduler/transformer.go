package main

import (
	"log"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

var algo = randomFit

type pendingTask struct {
	id         string
	endpoint   string
	expiration time.Time
	cfg        agent.ContainerConfig
}

func (pt pendingTask) isExpired() bool {
	return now().After(pt.expiration)
}

type transformer struct {
	quitc       chan chan struct{}
	scheduled   map[string]pendingTask
	unscheduled map[string]pendingTask
	ttl         time.Duration
	workQueue   *workQueue
}

func newTransformer(actual actualBroadcaster, desired desiredBroadcaster, target taskScheduler, ttl time.Duration) *transformer {
	t := &transformer{
		quitc:       make(chan chan struct{}),
		scheduled:   map[string]pendingTask{},
		unscheduled: map[string]pendingTask{},
		ttl:         ttl,
		workQueue:   newWorkQueue(),
	}
	go t.loop(actual, desired, target)
	return t
}

func (tr *transformer) quit() {
	q := make(chan struct{})
	tr.quitc <- q
	<-q
}

func (tr *transformer) loop(actual actualBroadcaster, desired desiredBroadcaster, target taskScheduler) {
	var (
		ticker   = time.NewTicker(tr.ttl)
		actualc  = make(chan map[string]agentState)
		desiredc = make(chan map[string]configstore.JobConfig)
		have     = map[string]agentState{}
		want     = map[string]configstore.JobConfig{}
	)

	defer ticker.Stop()

	actual.subscribe(actualc)
	defer actual.unsubscribe(actualc)

	select {
	case have = <-actualc:
	case <-after(time.Millisecond):
		panic("misbehaving actual-state broadcaster")
	}

	desired.subscribe(desiredc)
	defer desired.unsubscribe(desiredc)

	select {
	case want = <-desiredc:
	case <-after(time.Millisecond):
		panic("misbehaving desired-state broadcaster")
	}

	for {
		target, have, want := target, have, want
		select {
		case <-ticker.C:
			tr.workQueue.push(func() { tr.transform(want, have, target) })
		case want = <-desiredc:
			tr.workQueue.push(func() { tr.transform(want, have, target) })
		case have = <-actualc:
			tr.workQueue.push(func() { tr.transform(want, have, target) })
		case q := <-tr.quitc:
			tr.workQueue.quit()
			close(q)
			return
		}
	}
}

func (tr *transformer) transform(
	wantJobs map[string]configstore.JobConfig,
	agentStates map[string]agentState,
	target taskScheduler,
) {
	var (
		todo        = []func() error{}
		wantTasks   = map[string]agent.ContainerConfig{}
		haveTasks   = map[string]agent.ContainerConfig{}
		toSchedule  = map[string]agent.ContainerConfig{}
		id2endpoint = map[string]string{}
	)

	for _, cfg := range wantJobs {
		for i := 0; i < cfg.Scale; i++ {
			wantTasks[makeContainerID(cfg, i)] = cfg.ContainerConfig
		}
	}

	for endpoint, state := range agentStates {
		for id, instance := range state.instances {
			haveTasks[id] = instance.ContainerConfig
			id2endpoint[id] = endpoint
		}
	}

	tr.purge(id2endpoint)

	// Anything we want but don't have should be started.
	for id, cfg := range wantTasks {
		if _, ok := haveTasks[id]; ok {
			delete(wantTasks, id)
			delete(haveTasks, id)

			if _, isScheduled := tr.scheduled[id]; isScheduled {
				delete(tr.scheduled, id) // task is scheduled successfully
			}

			// we want the task and we have it, task state:
			//1) not pending: perfect!
			//2) pending to be unscheduled but now we want it again
			//   wait for the next pass to be unschedule it again
			continue
		}

		if _, isUnscheduled := tr.unscheduled[id]; isUnscheduled {
			delete(tr.unscheduled, id) // task is unscheduled
		}

		if _, isScheduled := tr.scheduled[id]; !isScheduled {
			toSchedule[id] = cfg
		}
	}

	mapping, unscheduled := algo(toSchedule, agentStates, tr.scheduled)
	if len(unscheduled) > 0 {
		log.Printf("transformer: error unscheduled tasks: %v", unscheduled)
		// TODO(pb): do something else?
	}

	for endpoint, cfgs := range mapping {
		for id, cfg := range cfgs {
			tr.scheduled[id] = pendingTask{
				id:         id,
				endpoint:   endpoint,
				cfg:        cfg,
				expiration: now().Add(tr.ttl),
			}

			var endpoint, id, cfg = endpoint, id, cfg
			todo = append(todo, func() error {
				return target.schedule(endpoint, id, cfg)
			})
		}
	}

	// Anything we have but don't want should be stopped.
	for id, cfg := range haveTasks {
		endpoint, ok := id2endpoint[id]
		if !ok {
			panic("invalid state in transform")
		}

		if _, ok := tr.unscheduled[id]; ok {
			continue
		}

		tr.unscheduled[id] = pendingTask{
			id:         id,
			endpoint:   endpoint,
			cfg:        cfg,
			expiration: now().Add(tr.ttl),
		}

		var id = id
		todo = append(todo, func() error {
			return target.unschedule(endpoint, id)
		})
	}

	// Engage.
	for _, fn := range todo {
		if err := fn(); err != nil {
			log.Printf("transformer: error during transform: %s", err)
			continue
		}
	}
}

func (tr *transformer) purge(have map[string]string) {
	for id, task := range tr.scheduled {
		if task.isExpired() {
			delete(tr.scheduled, id)
		}
	}

	for id, task := range tr.unscheduled {
		if _, ok := have[id]; !ok || task.isExpired() {
			delete(tr.unscheduled, id)
		}
	}
}
