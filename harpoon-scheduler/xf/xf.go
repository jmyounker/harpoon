// Package xf deals with the transform of registry state to mutations against
// a set of agents.
package xf

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
	"github.com/soundcloud/harpoon/harpoon-scheduler/algo"
	"github.com/soundcloud/harpoon/harpoon-scheduler/metrics"
	"github.com/soundcloud/harpoon/harpoon-scheduler/xtime"
)

var (
	// Debugf may be set from a controlling package.
	Debugf = func(string, ...interface{}) {}

	// Tolerance is the time we're willing to wait for an individual mutation
	// command against a task scheduler (i.e. an agent) to take effect, before
	// we give up and repeat the command.
	Tolerance = 1 * time.Minute

	// Algorithm is the scheduling algorithm we'll use when placing new
	// containers.
	Algorithm = algo.RandomFit

	// tickInterval is how often the Transform will attempt to reconcile
	// desired and actual states, absent a mutation event. Basically, every
	// time this fires, we'll retry failed mutations.
	tickInterval = 3 * time.Second
)

// DesireBroadcaster emits the complete desired state of the scheduling domain
// whenever any element in the domain changes. All broadcasters emit their
// current state to every newly-subscribed channel as an initial message.
type DesireBroadcaster interface {
	Subscribe(chan<- map[string]configstore.JobConfig)
	Unsubscribe(chan<- map[string]configstore.JobConfig)
}

// ActualBroadcaster emits the complete actual state of the scheduling domain
// whenever any element in the domain changes. All broadcasters emit their
// current state to every newly-subscribed channel as an initial message.
type ActualBroadcaster interface {
	Subscribe(chan<- map[string]agent.StateEvent)
	Unsubscribe(chan<- map[string]agent.StateEvent)
}

// TaskScheduler is any component which can accept schedule commands for
// individual containers, known as tasks in the scheduler lingo.
type TaskScheduler interface {
	Schedule(endpoint, id string, config agent.ContainerConfig) error
	Unschedule(endpoint, id string) error
}

// Transform continuously monitors the desire- and actual-broadcasters, and
// emits appropriate mutations to the task scheduler. It never returns.
func Transform(
	desire DesireBroadcaster,
	actual ActualBroadcaster,
	target TaskScheduler,
) {
	var (
		desirec = make(chan map[string]configstore.JobConfig)
		actualc = make(chan map[string]agent.StateEvent)
		want    = map[string]configstore.JobConfig{}
		have    = map[string]agent.StateEvent{}
		pending = map[string]algo.PendingTask{}
		tick    = time.Tick(tickInterval)
	)

	desire.Subscribe(desirec)
	defer desire.Unsubscribe(desirec)

	select {
	case want = <-desirec:
	case <-time.After(time.Millisecond):
		panic("misbehaving desire broadcaster")
	}

	actual.Subscribe(actualc)
	defer actual.Unsubscribe(actualc)

	select {
	case have = <-actualc:
	case <-time.After(time.Millisecond):
		panic("misbehaving actual broadcaster")
	}

	// We need to execute the transform asynchronously, so that desire and
	// actual events don't block or deadlock. But, we want at-most-once
	// execution of transform. So, we use a semaphore, and say that  any
	// transform-triggering event that happens while we're already
	// transforming will get picked up on the next trigger.
	var (
		semaphore    = make(chan bool, 1)
		tryTransform = func(want map[string]configstore.JobConfig, have map[string]agent.StateEvent) {
			select {
			case semaphore <- true:
				Debugf("tryTransform success")
				pending = transform(want, have, target, pending)
				metrics.IncTransformsExecuted(1)
				<-semaphore

			default:
				Debugf("tryTransform skipped")
				metrics.IncTransformsSkipped(1)
			}
		}
	)

	for {
		select {
		case want = <-desirec:
			Debugf("actual state change")
			go tryTransform(want, have)

		case have = <-actualc:
			Debugf("desire state change")
			go tryTransform(want, have)

		case <-tick:
			Debugf("tick")
			go tryTransform(want, have)
		}
	}
}

// transform compares the desired (want) and actual (have) states of the
// scheduling domain, reconciles them with the outstanding mutations
// (pending), and issues any necessary mutations to the task scheduler
// (target).
//
// This function must return quickly. It only issues mutation commands; it
// doesn't wait for them to take effect. Consequently, the target
// taskScheduler's methods must return quickly as well.
func transform(
	want map[string]configstore.JobConfig,
	have map[string]agent.StateEvent,
	target TaskScheduler,
	pending map[string]algo.PendingTask,
) map[string]algo.PendingTask {
	var (
		wantTasks    = map[string]agent.ContainerConfig{}
		haveTasks    = map[string]agent.ContainerConfig{}
		id2endpoints = map[string][]string{} // detected
		id2endpoint  = map[string]string{}   // blessed
		toSchedule   = map[string]agent.ContainerConfig{}
		toUnschedule = map[string][]string{}
	)

	// Expand every wanted Job to its composite tasks.
	for _, config := range want {
		for i := 0; i < config.Scale; i++ {
			wantTasks[makeContainerID(config.Hash(), i)] = config.ContainerConfig
		}
	}

	// Index every running container by its ID.
	for endpoint, state := range have {
		for id, instance := range state.Containers {
			if instance.ContainerStatus == agent.ContainerStatusRunning {
				haveTasks[id] = instance.ContainerConfig
				id2endpoints[id] = append(id2endpoints[id], endpoint)
			}
		}
	}

	// It's possible we have the same container ID running on different
	// endpoints. That's not allowed.
	for id, endpoints := range id2endpoints {
		if len(endpoints) == 0 {
			panic("bad state in transform duplicate detection")
		}

		if len(endpoints) == 1 {
			id2endpoint[id] = endpoints[0]
			continue
		}

		log.Printf("%s: duplicate detected: %d instance(s) total", id, len(endpoints))
		blessed, others := chooseOne(endpoints)
		log.Printf("%s: keeping instance on %s", id, blessed)
		id2endpoint[id] = blessed
		for _, endpoint := range others {
			log.Printf("%s: unscheduling duplicate on %s", id, endpoint)
			toUnschedule[endpoint] = append(toUnschedule[endpoint], id)
		}
	}

	// Purge the pending tasks that have been satisfied, or have reached their
	// deadline. If they're expired, this just means that we'll re-execute
	// their mutations. They'll never get purged from the desired state (the
	// registry).
	for id, p := range pending {
		if _, ok := haveTasks[id]; ok && p.Schedule {
			Debugf("pending task %q successfully scheduled; delete from pending", id)
			delete(pending, id) // successful schedule
		} else if !ok && !p.Schedule {
			Debugf("pending task %q successfully unscheduled; delete from pending", id)
			delete(pending, id) // successful unschedule
		} else if xtime.Now().After(p.Deadline) {
			Debugf("pending task %q expired; delete from pending", id)
			delete(pending, id) // timeout
		}
	}

	Debugf(
		"want %d task(s), have %d task(s), pending %d task(s)",
		len(wantTasks),
		len(haveTasks),
		len(pending),
	)

	// Anything we want, but don't have, should be started.
	for id, config := range wantTasks {
		if _, ok := haveTasks[id]; ok {
			delete(wantTasks, id)
			delete(haveTasks, id)

			if pendingTask, ok := pending[id]; ok {
				switch pendingTask.Schedule {
				case true:
					delete(pending, id)
				case false:
					panic(fmt.Sprintf("%q is pending unschedule, but exists in both the registry and the actual state: strange state!", id))
				}
			}

			continue
		}

		if pendingTask, ok := pending[id]; ok {
			switch pendingTask.Schedule {
			case true:
				continue // already pending schedule; don't reissue command
			case false:
				panic(fmt.Sprintf("%q is pending unschedule, but exists in the registry: strange state!", id))
			}
		}

		toSchedule[id] = config
	}

	// Schedule those containers that need it.
	placed, failed := Algorithm(toSchedule, have, pending)
	if len(failed) > 0 {
		log.Printf("the scheduling algorithm failed to place %d/%d tasks", len(failed), len(toSchedule))
	}

	metrics.IncContainersRequested(len(toSchedule))
	metrics.IncContainersPlaced(len(placed))
	metrics.IncContainersFailed(len(failed))

	// Invoke the schedule mutations.
	for endpoint, configs := range placed {
		for id, config := range configs {
			if err := target.Schedule(endpoint, id, config); err != nil {
				log.Printf("%s schedule %q failed: %s", endpoint, id, err)
				continue
			}

			Debugf("%s schedule %q now pending", endpoint, id)
			pending[id] = algo.PendingTask{
				Schedule:        true,
				Deadline:        xtime.Now().Add(Tolerance),
				Endpoint:        endpoint,
				ContainerConfig: config,
			} // we issued the mutation
		}
	}

	// Anything we have, but don't want, should be stopped. (Everything left
	// in haveTasks is unrepresented in wantTasks, and therefore unwanted.)
	for id := range haveTasks {
		endpoint, ok := id2endpoint[id]
		if !ok {
			panic(fmt.Sprintf("can't reverse-lookup container %q", id))
		}

		if pendingTask, ok := pending[id]; ok {
			switch pendingTask.Schedule {
			case true:
				panic(fmt.Sprintf("strange state in Transform"))
			case false:
				continue // already pending unschedule; don't reissue command
			}
		}

		toUnschedule[endpoint] = append(toUnschedule[endpoint], id)
	}

	// Invoke the unschedule mutations.
	for endpoint, ids := range toUnschedule {
		for _, id := range ids {
			if err := target.Unschedule(endpoint, id); err != nil {
				log.Printf("%s unschedule %q failed: %s", endpoint, id, err)
				continue
			}

			Debugf("%s unschedule %q now pending", endpoint, id)
			pending[id] = algo.PendingTask{
				Schedule: false,
				Deadline: xtime.Now().Add(Tolerance),
			} // we issued the mutation
		}
	}

	return pending
}

func chooseOne(a []string) (string, []string) {
	idx := rand.Intn(len(a))
	return a[idx], append(a[:idx], a[idx+1:]...)
}
