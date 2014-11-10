// Package xf deals with the transform of registry state to mutations against
// a set of agents.
package xf

import (
	"fmt"
	"log"
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
	// execution of transform. So, we use a semaphore, and say that any
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
			Debugf("desire state change")
			go tryTransform(want, have)

		case have = <-actualc:
			Debugf("actual state change")
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
// TaskScheduler's methods must return quickly as well.
func transform(
	want map[string]configstore.JobConfig,
	have map[string]agent.StateEvent,
	target TaskScheduler,
	pending map[string]algo.PendingTask,
) map[string]algo.PendingTask {
	var (
		wantTasks    = map[string]agent.ContainerConfig{}              // id: config
		haveTasks    = map[string]map[string]agent.ContainerInstance{} // id: endpoint: instance
		toKeep       = map[string]string{}                             // endpoint: id
		toSchedule   = map[string]agent.ContainerConfig{}              // id: config
		toStart      = map[string]map[string]agent.ContainerConfig{}   // endpoint: configs
		toUnschedule = map[string][]string{}                           // endpoint: ids
	)

	// Expand every wanted Job to its composite tasks.
	for _, config := range want {
		for i := 0; i < config.Scale; i++ {
			wantTasks[makeContainerID(config.Hash(), i)] = config.ContainerConfig
		}
	}

	// Index every running container by its ID. It's possible we'll get some
	// duplicate containers in the overall domain. We'll deal with them later.
	for endpoint, state := range have {
		for id, instance := range state.Containers {
			m, ok := haveTasks[id]
			if !ok {
				m = map[string]agent.ContainerInstance{}
			}

			if _, ok = m[endpoint]; ok {
				panic(fmt.Sprintf("during transform, duplicate container %q on %s", id, endpoint))
			}

			m[endpoint] = instance
			haveTasks[id] = m
		}
	}

	// Clean up any pending tasks. That means purging those that have already
	// been satisfied, or reached their deadline. If they're expired, this
	// just means that we'll re-execute their mutations. They'll never get
	// purged from the desired state (the registry).

	has := func(m map[string]agent.ContainerInstance, valid ...agent.ContainerStatus) bool {
		v := map[agent.ContainerStatus]struct{}{}
		for _, s := range valid {
			v[s] = struct{}{}
		}

		for _, i := range m {
			if _, ok := v[i.ContainerStatus]; ok {
				return true
			}
		}

		return false
	}

	for id, p := range pending {
		if m, ok := haveTasks[id]; ok && p.Schedule && has(m,
			agent.ContainerStatusRunning,
			agent.ContainerStatusFinished,
			agent.ContainerStatusFailed,
		) {
			Debugf("pending task %q successfully scheduled; delete from pending", id)
			delete(pending, id) // successful schedule
		} else if !p.Schedule && (!ok || !has(m,
			agent.ContainerStatusCreated,
			agent.ContainerStatusRunning,
			agent.ContainerStatusFinished,
			agent.ContainerStatusFailed,
		)) {
			Debugf("pending task %q successfully unscheduled; delete from pending", id)
			delete(pending, id) // successful unschedule
		} else if xtime.Now().After(p.Deadline) {
			Debugf("pending task %q expired; delete from pending", id)
			delete(pending, id) // timeout
		}
	}

	Debugf(
		"before scan: want %d task(s), have %d task(s), pending %d task(s)",
		len(wantTasks),
		len(haveTasks), // maybe includes duplicates
		len(pending),
	)

	// The scheduler issues the initial command(s), but then leaves the
	// container alone. The restart behavior is parsed only by the agent. As
	// long as a container exists, in any state, the scheduler considers it
	// "running".
	//
	// So: walk our wantTasks, and try to locate each in our haveTasks. If we
	// find a running, finished, or failed instance, great: keep it, and
	// unschedule the rest. If we find a created instance, and it's not
	// pending-schedule, then we'll assume the Start signal was lost, and
	// issue another schedule mutation. Otherwise, schedule a new instance.

	// Scan the domain for instances we can keep.
	for id, config := range wantTasks {
		if _, ok := haveTasks[id]; ok {
			if len(haveTasks[id]) == 0 {
				panic(fmt.Sprintf("existing task %q runs on 0 agents", id))
			}

			// This wanted task is already under supervision somewhere, maybe
			// more than once. Pick one of those instances and use it, rather
			// than scheduling a new one.

			var (
				satisfied = false
			)
			for endpoint, instance := range haveTasks[id] {
				if satisfied {
					// The wanted container has already been satisfied
					// elsewhere in the domain. Remove this instance.
					delete(haveTasks[id], endpoint) // accounted-for
					toUnschedule[endpoint] = append(toUnschedule[endpoint], id)
					continue
				}

				var (
					created         = instance.ContainerStatus == agent.ContainerStatusCreated
					running         = instance.ContainerStatus == agent.ContainerStatusRunning
					finished        = instance.ContainerStatus == agent.ContainerStatusFinished
					failed          = instance.ContainerStatus == agent.ContainerStatusFailed
					pendingSchedule = func() bool { b, ok := pending[id]; return ok && b.Schedule }()
				)

				if running || finished || failed {
					// The container is already being supervised.
					delete(haveTasks[id], endpoint) // accounted-for
					toKeep[endpoint] = id
					satisfied = true
					continue
				}

				if created && pendingSchedule {
					// This instance is apparently in the process of being
					// started. We'll just wait for it, and unschedule the
					// others.
					delete(haveTasks[id], endpoint) // accounted-for
					toKeep[endpoint] = id
					satisfied = true
					continue
				}

				if created && !pendingSchedule {
					// The Start signal was lost? I don't know how else it
					// would stick around in Created state. Keep it, and also
					// re-emit the scheduling signal.
					delete(haveTasks[id], endpoint) // accounted-for
					toKeep[endpoint] = id
					{
						m, ok := toStart[endpoint]
						if !ok {
							m = map[string]agent.ContainerConfig{}
						}
						m[id] = config
						toStart[endpoint] = m
					}
					satisfied = true
					continue
				}

				panic(fmt.Sprintf("unreachable: Status %q pendingSchedule %v", instance.ContainerStatus, pendingSchedule))
			}

			if !satisfied {
				panic(fmt.Sprintf("no extant instance of task %q was marked as satisfactory", id))
			}

			if n := len(haveTasks[id]); n != 0 {
				panic(fmt.Sprintf("after scan, %d instance(s) of %q remain unaccounted-for", n, id))
			}

			delete(wantTasks, id) // accounted-for
			continue
		}

		// A new task, so we probably need to schedule. But first, check if
		// it's not already pending-schedule.
		if p, ok := pending[id]; ok && p.Schedule {
			delete(wantTasks, id) // accounted-for
			continue
		} else if ok && !p.Schedule {
			panic(fmt.Sprintf("%q is pending unschedule, but exists in the registry", id))
		}

		// Good.
		delete(wantTasks, id)
		toSchedule[id] = config
	}

	// Everything left in haveTasks is unrepresented in wantTasks, and
	// therefore unwanted. And anything we don't want should be unscheduled.
	for id, endpoints := range haveTasks {
		for endpoint := range endpoints {
			if p, ok := pending[id]; ok && !p.Schedule {
				continue // already pending-unschedule, so that's fine
			} else if ok && p.Schedule {
				panic(fmt.Sprintf("%q is pending schedule after scan; strange state", id))
			}

			toUnschedule[endpoint] = append(toUnschedule[endpoint], id)
		}
	}

	var killCount int
	for _, endpoints := range toUnschedule {
		killCount += len(endpoints)
	}

	Debugf(
		"after scan: %d to keep, %d to schedule, %d to unschedule",
		len(toKeep),
		len(toSchedule),
		killCount,
	)

	// Schedule those containers that need it.
	placed, failed := Algorithm(toSchedule, have, pending)
	if len(failed) > 0 {
		log.Printf("the scheduling algorithm failed to place %d/%d tasks", len(failed), len(toSchedule))
	}

	metrics.IncContainersRequested(len(toSchedule))
	metrics.IncContainersPlaced(len(placed))
	metrics.IncContainersFailed(len(failed))

	// Invoke the schedule mutations.
	sched := func(m map[string]map[string]agent.ContainerConfig) {
		for endpoint, configs := range m {
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
	}
	sched(toStart)
	sched(placed)

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
				Endpoint: endpoint,
			} // we issued the mutation
		}
	}

	return pending
}
