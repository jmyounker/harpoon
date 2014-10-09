package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"reflect"
	"testing"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

func TestTransform(t *testing.T) {
	log.SetOutput(ioutil.Discard)
	//log.SetFlags(log.Lmicroseconds)

	var (
		job                = "professor-wiggles"
		jobScaled          = "professor-wiggles-scaled"
		cfg                = configstore.JobConfig{Job: job, Scale: 1}
		factor             = 4
		cfgScaled          = configstore.JobConfig{Job: jobScaled, Scale: factor}
		containerCfgs      = make(map[string]agent.ContainerConfig, factor)
		endpoint           = "http://some.arbitrary.endpoint.info:9191"
		id                 = makeContainerID(cfg, 0) // just one task
		stopped            = make(map[string]struct{}, factor)
		containerInstances = make(map[string]agent.ContainerInstance, factor)
	)

	for i := 0; i < factor; i++ {
		cfgID := makeContainerID(cfgScaled, i)
		containerCfgs[cfgID] = agent.ContainerConfig{}
		stopped[cfgID] = struct{}{}
		containerInstances[cfgID] = agent.ContainerInstance{}
	}

	type testCase struct {
		wantJobs      map[string]configstore.JobConfig
		haveInstances map[string]agentState
		started       map[string]map[string]agent.ContainerConfig
		stopped       map[string]map[string]struct{}
		countStarted  int
		countStopped  int
	}

	for i, input := range []testCase{
		{
			wantJobs:      map[string]configstore.JobConfig{},
			haveInstances: map[string]agentState{},
			started:       map[string]map[string]agent.ContainerConfig{},
			stopped:       map[string]map[string]struct{}{},
			countStarted:  0,
			countStopped:  0,
		},
		{
			wantJobs:      map[string]configstore.JobConfig{job: cfg},
			haveInstances: map[string]agentState{endpoint: agentState{instances: map[string]agent.ContainerInstance{}}},
			started:       map[string]map[string]agent.ContainerConfig{endpoint: map[string]agent.ContainerConfig{id: agent.ContainerConfig{}}},
			stopped:       map[string]map[string]struct{}{},
			countStarted:  1,
			countStopped:  0,
		},
		{
			wantJobs:      map[string]configstore.JobConfig{job: cfg},
			haveInstances: map[string]agentState{endpoint: agentState{instances: map[string]agent.ContainerInstance{id: agent.ContainerInstance{}}}},
			started:       map[string]map[string]agent.ContainerConfig{},
			stopped:       map[string]map[string]struct{}{},
			countStarted:  0,
			countStopped:  0,
		},
		{
			wantJobs:      map[string]configstore.JobConfig{},
			haveInstances: map[string]agentState{endpoint: agentState{instances: map[string]agent.ContainerInstance{job: agent.ContainerInstance{}}}},
			started:       map[string]map[string]agent.ContainerConfig{},
			stopped:       map[string]map[string]struct{}{endpoint: map[string]struct{}{job: struct{}{}}},
			countStarted:  0,
			countStopped:  1,
		},
		{
			wantJobs:      map[string]configstore.JobConfig{},
			haveInstances: map[string]agentState{endpoint: agentState{instances: containerInstances}},
			started:       map[string]map[string]agent.ContainerConfig{},
			stopped:       map[string]map[string]struct{}{endpoint: stopped},
			countStarted:  0,
			countStopped:  4,
		},
		{
			wantJobs:      map[string]configstore.JobConfig{job: cfgScaled},
			haveInstances: map[string]agentState{endpoint: agentState{instances: map[string]agent.ContainerInstance{}}},
			started:       map[string]map[string]agent.ContainerConfig{endpoint: containerCfgs},
			stopped:       map[string]map[string]struct{}{},
			countStarted:  4,
			countStopped:  0,
		},
		{
			wantJobs: map[string]configstore.JobConfig{job: cfgScaled},
			haveInstances: map[string]agentState{
				endpoint: agentState{
					instances: map[string]agent.ContainerInstance{
						id: agent.ContainerInstance{},
						makeContainerID(cfgScaled, 2): agent.ContainerInstance{},
					},
				},
			},
			started: map[string]map[string]agent.ContainerConfig{
				endpoint: map[string]agent.ContainerConfig{
					makeContainerID(cfgScaled, 0): agent.ContainerConfig{},
					makeContainerID(cfgScaled, 1): agent.ContainerConfig{},
					makeContainerID(cfgScaled, 3): agent.ContainerConfig{},
				},
			},
			stopped:      map[string]map[string]struct{}{endpoint: map[string]struct{}{id: struct{}{}}},
			countStarted: 3,
			countStopped: 1,
		},
	} {
		var (
			target = newMockTaskScheduler()
			tr     = transformer{
				ttl:         time.Second * 10,
				scheduled:   map[string]pendingTask{},
				unscheduled: map[string]pendingTask{},
			}
		)

		tr.transform(input.wantJobs, input.haveInstances, target)
		want, have := input.started, target.started
		if !reflect.DeepEqual(want, have) {
			t.Errorf("%d: started: want %v, have %v", i, want, have)
		}

		if want, have := input.countStarted, len(tr.scheduled); want != have {
			t.Errorf("%d: pending scheduled: want %v, have %v", i, want, have)
		}

		scheduled := map[string]pendingTask{}
		for id, task := range tr.scheduled {
			scheduled[id] = task
			if task.isExpired() {
				t.Errorf("%d: task %q should not be expired", i, id)
			}
		}

		if want, have := input.stopped, target.stopped; !reflect.DeepEqual(want, have) {
			t.Errorf("%d: stopped: want %v, have %v", i, want, have)
		}

		unscheduled := map[string]pendingTask{}
		for id, task := range tr.unscheduled {
			unscheduled[id] = task
			if task.isExpired() {
				t.Errorf("%d: task %q should not be expired", i, id)
			}
		}

		if want, have := input.countStopped, len(tr.unscheduled); want != have {
			t.Errorf("%d: pending unscheduled: want %v, have %v", i, want, have)
		}

		target = newMockTaskScheduler()
		newJob := configstore.JobConfig{Job: "newJob", Scale: 2}
		input.wantJobs["newJob"] = newJob
		id1 := makeContainerID(newJob, 0)
		id2 := makeContainerID(newJob, 1)

		tr.transform(input.wantJobs, input.haveInstances, target)
		started := map[string]map[string]agent.ContainerConfig{}
		if len(input.haveInstances) > 0 {
			started[endpoint] = map[string]agent.ContainerConfig{
				id1: agent.ContainerConfig{},
				id2: agent.ContainerConfig{},
			}
		}

		if want, have := started, target.started; !reflect.DeepEqual(want, have) {
			t.Errorf("%d: pending scheduled: want %v, have %v", i, want, have)
		}

		if want, have := 0, len(target.stopped); want != have {
			t.Errorf("%d: stopped: want %v, have %v", i, want, have)
		}

		if len(input.haveInstances) > 0 {
			if task, ok := tr.scheduled[id1]; !ok || task.isExpired() {
				t.Errorf("%d: not properly set pending state of scheduled task", i)
			}
			delete(tr.scheduled, id1)

			if task, ok := tr.scheduled[id2]; !ok || task.isExpired() {
				t.Errorf("%d: not properly set pending state of scheduled task", i)
			}
			delete(tr.scheduled, id2)
		}

		if !reflect.DeepEqual(tr.scheduled, scheduled) {
			t.Errorf("scheduled tasks should not have changed")
		}
		if !reflect.DeepEqual(tr.unscheduled, unscheduled) {
			t.Errorf("unscheduled tasks should not have changed")
		}
	}
}

func TestPurgePengingTasks(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		tr = transformer{
			scheduled:   map[string]pendingTask{},
			unscheduled: map[string]pendingTask{},
		}
		taskCount = 10
		have      = map[string]string{}
	)

	for i := 0; i < taskCount; i++ {
		tr.scheduled[fmt.Sprintf("expired%d", i)] = pendingTask{
			expiration: time.Now(),
		}
		tr.unscheduled[fmt.Sprintf("expiredU%d", i)] = pendingTask{
			expiration: time.Now(),
		}
		tr.scheduled[fmt.Sprintf("valid%d", i)] = pendingTask{
			expiration: time.Now().Add(time.Minute),
		}
		id := fmt.Sprintf("validU%d", i)
		tr.unscheduled[id] = pendingTask{
			expiration: time.Now().Add(time.Minute),
		}
		have[id] = "endpoint"

		tr.unscheduled[fmt.Sprintf("successfullyUnscheduled%d", i)] = pendingTask{
			expiration: time.Now().Add(time.Minute),
		}
	}

	tr.purge(have)

	if want, have := taskCount, len(tr.scheduled); want != have {
		t.Errorf("Purge not valid for scheduled task! expected %d != actual %d", want, have)
	}

	if want, have := taskCount, len(tr.unscheduled); want != have {
		t.Errorf("Purge not valid for unscheduled task! expected %d != actual %d", want, have)
	}
}

func TestTransformExpired(t *testing.T) {
	log.SetOutput(ioutil.Discard)
	//log.SetFlags(log.Lmicroseconds)

	var (
		job      = "professor-wiggles"
		cfg      = configstore.JobConfig{Job: job, Scale: 1}
		id       = makeContainerID(cfg, 0) // just one task
		endpoint = "http://some.arbitrary.endpoint.info:9191"
	)

	type testCase struct {
		wantJobs      map[string]configstore.JobConfig
		haveInstances map[string]agentState
		started       map[string]map[string]agent.ContainerConfig
		stopped       map[string]map[string]struct{}
		countStarted  int
		countStopped  int
	}

	for i, input := range []testCase{
		{
			wantJobs:      map[string]configstore.JobConfig{job: cfg},
			haveInstances: map[string]agentState{endpoint: agentState{instances: map[string]agent.ContainerInstance{}}},
			started:       map[string]map[string]agent.ContainerConfig{endpoint: map[string]agent.ContainerConfig{id: agent.ContainerConfig{}}},
			stopped:       map[string]map[string]struct{}{},
			countStarted:  1,
			countStopped:  0,
		},
		{
			wantJobs:      map[string]configstore.JobConfig{job: cfg},
			haveInstances: map[string]agentState{endpoint: agentState{instances: map[string]agent.ContainerInstance{id: agent.ContainerInstance{}}}},
			started:       map[string]map[string]agent.ContainerConfig{},
			stopped:       map[string]map[string]struct{}{},
			countStarted:  0,
			countStopped:  0,
		},
		{
			wantJobs:      map[string]configstore.JobConfig{},
			haveInstances: map[string]agentState{endpoint: agentState{instances: map[string]agent.ContainerInstance{job: agent.ContainerInstance{}}}},
			started:       map[string]map[string]agent.ContainerConfig{},
			stopped:       map[string]map[string]struct{}{endpoint: map[string]struct{}{job: struct{}{}}},
			countStarted:  0,
			countStopped:  1,
		},
	} {
		var (
			target = newMockTaskScheduler()
			tr     = transformer{
				ttl:         time.Second * 10,
				scheduled:   map[string]pendingTask{},
				unscheduled: map[string]pendingTask{},
			}
		)

		target = newMockTaskScheduler()
		for _, instances := range input.started {
			for id := range instances {
				tr.scheduled[id] = pendingTask{expiration: time.Now()}
			}
		}
		tr.scheduled["random"] = pendingTask{expiration: time.Now()}
		for _, instances := range input.stopped {
			for id := range instances {
				tr.unscheduled[id] = pendingTask{expiration: time.Now()}
			}
		}
		tr.unscheduled["random"] = pendingTask{expiration: time.Now()}
		tr.transform(input.wantJobs, input.haveInstances, target)
		want, have := input.started, target.started
		if !reflect.DeepEqual(want, have) {
			t.Errorf("%d: started: want %v, have %v", i, want, have)
		}

		if want, have := input.countStarted, len(tr.scheduled); want != have {
			t.Errorf("%d: pending scheduled: want %v, have %v", i, want, have)
		}

		for id, task := range tr.scheduled {
			if task.isExpired() {
				t.Errorf("%d: task %q should not be expired", i, id)
			}
		}

		if want, have := input.stopped, target.stopped; !reflect.DeepEqual(want, have) {
			t.Errorf("%d: stopped: want %v, have %v", i, want, have)
		}

		for id, task := range tr.unscheduled {
			if task.isExpired() {
				t.Errorf("%d: task %q should not be expired", i, id)
			}
		}

		if want, have := input.countStopped, len(tr.unscheduled); want != have {
			t.Errorf("%d: pending unscheduled: want %v, have %v", i, want, have)
		}
	}
}

func TestTaskSuccessfullyUnscheduled(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		job           = "professor-wiggles-scaled"
		cfg           = configstore.JobConfig{Job: job, Scale: 1}
		wantJobs      = map[string]configstore.JobConfig{job: cfg}
		id            = makeContainerID(cfg, 0)
		endpoint      = "state2"
		haveInstances = map[string]agentState{
			endpoint: agentState{
				instances: map[string]agent.ContainerInstance{
					id: agent.ContainerInstance{},
				},
			},
		}

		target = newMockTaskScheduler()
		tr     = transformer{
			ttl: time.Second * 10,
			scheduled: map[string]pendingTask{
				id: pendingTask{id: id, endpoint: endpoint, cfg: cfg.ContainerConfig, expiration: time.Now().Add(time.Second)},
			},
			unscheduled: map[string]pendingTask{},
		}
	)
	tr.transform(wantJobs, haveInstances, target)
	if err := validate(target, tr, 0, 0, 0, 0); err != nil {
		t.Error(err)
	}
}

func TestTaskNotScheduledYet(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		job           = "professor-wiggles-scaled"
		cfg           = configstore.JobConfig{Job: job, Scale: 1}
		wantJobs      = map[string]configstore.JobConfig{job: cfg}
		id            = makeContainerID(cfg, 0)
		endpoint      = "state2"
		haveInstances = map[string]agentState{
			endpoint: agentState{
				instances: map[string]agent.ContainerInstance{},
			},
		}

		target = newMockTaskScheduler()
		tr     = transformer{
			ttl: time.Second * 10,
			scheduled: map[string]pendingTask{
				id: pendingTask{id: id, endpoint: endpoint, cfg: cfg.ContainerConfig, expiration: time.Now().Add(time.Second)},
			},
			unscheduled: map[string]pendingTask{},
		}
	)
	tr.transform(wantJobs, haveInstances, target)
	if err := validate(target, tr, 1, 0, 0, 0); err != nil {
		t.Error(err)
	}
}

func TestPendingTask(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		wantJobs      = map[string]configstore.JobConfig{}
		endpoint      = "state2"
		haveInstances = map[string]agentState{
			endpoint: agentState{
				instances: map[string]agent.ContainerInstance{},
			},
		}

		target = newMockTaskScheduler()
		tr     = transformer{
			ttl: time.Second * 10,
			scheduled: map[string]pendingTask{
				"id": pendingTask{id: "asd", endpoint: "asd", cfg: agent.ContainerConfig{}, expiration: time.Now().Add(time.Second)},
			},
			unscheduled: map[string]pendingTask{},
		}
	)
	tr.transform(wantJobs, haveInstances, target)
	if err := validate(target, tr, 1, 0, 0, 0); err != nil {
		t.Error(err)
	}
}

func TestEverythingOK(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		job           = "professor-wiggles-scaled"
		cfg           = configstore.JobConfig{Job: job, Scale: 2}
		wantJobs      = map[string]configstore.JobConfig{job: cfg}
		id1           = makeContainerID(cfg, 0)
		id2           = makeContainerID(cfg, 1)
		haveInstances = map[string]agentState{
			"state2": agentState{
				instances: map[string]agent.ContainerInstance{
					id1: agent.ContainerInstance{},
					id2: agent.ContainerInstance{},
				},
			},
		}

		target = newMockTaskScheduler()
		tr     = transformer{
			ttl:         time.Second * 10,
			scheduled:   map[string]pendingTask{},
			unscheduled: map[string]pendingTask{},
		}
	)
	tr.transform(wantJobs, haveInstances, target)
	if err := validate(target, tr, 0, 0, 0, 0); err != nil {
		t.Error(err)
	}
}

func TestScheduleNeeded(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		job           = "professor-wiggles-scaled"
		cfg           = configstore.JobConfig{Job: job, Scale: 2}
		wantJobs      = map[string]configstore.JobConfig{job: cfg}
		id1           = makeContainerID(cfg, 0)
		id2           = makeContainerID(cfg, 1)
		endpoint      = "state2"
		haveInstances = map[string]agentState{
			"state2": agentState{
				instances: map[string]agent.ContainerInstance{
					id2: agent.ContainerInstance{},
				},
			},
		}

		target = newMockTaskScheduler()
		tr     = transformer{
			ttl:         time.Second * 10,
			scheduled:   map[string]pendingTask{},
			unscheduled: map[string]pendingTask{},
		}
	)
	tr.transform(wantJobs, haveInstances, target)
	if err := validate(target, tr, 1, 0, 1, 0); err != nil {
		t.Error(err)
	}

	task, ok := tr.scheduled[id1]
	if !ok {
		t.Errorf("Task %q should be scheduled", id1)
	}

	if task.isExpired() {
		t.Errorf("Task %q should not be expired", id1)
	}

	instances, ok := target.started[endpoint]
	if !ok {
		t.Errorf("Task %q shoudld be scheduled on %q", id1, endpoint)
	}

	if _, ok := instances[id1]; !ok {
		t.Errorf("Task %q shoudld be scheduled on %q", id1, endpoint)
	}
}

func TestUnscheduleNeeded(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		job           = "professor-wiggles-scaled"
		cfg           = configstore.JobConfig{Job: job, Scale: 2}
		wantJobs      = map[string]configstore.JobConfig{}
		id1           = makeContainerID(cfg, 0)
		endpoint      = "state2"
		haveInstances = map[string]agentState{
			"state2": agentState{
				instances: map[string]agent.ContainerInstance{
					id1: agent.ContainerInstance{},
				},
			},
		}

		target = newMockTaskScheduler()
		tr     = transformer{
			ttl:         time.Second * 10,
			scheduled:   map[string]pendingTask{},
			unscheduled: map[string]pendingTask{},
		}
	)
	tr.transform(wantJobs, haveInstances, target)
	if err := validate(target, tr, 0, 1, 0, 1); err != nil {
		t.Error(err)
	}

	task, ok := tr.unscheduled[id1]
	if !ok {
		t.Errorf("Task %q should be unscheduled", id1)
	}

	if task.isExpired() {
		t.Errorf("Task %q should not be expired", id1)
	}

	instances, ok := target.stopped[endpoint]
	if !ok {
		t.Errorf("Task %q shoudld be scheduled on %q", id1, endpoint)
	}

	if _, ok := instances[id1]; !ok {
		t.Errorf("Task %q shoudld be scheduled on %q", id1, endpoint)
	}
}

func TestUnscheduledPendingTask(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		job           = "professor-wiggles-scaled"
		cfg           = configstore.JobConfig{Job: job, Scale: 2}
		id1           = makeContainerID(cfg, 0)
		id2           = makeContainerID(cfg, 1)
		wantJobs      = map[string]configstore.JobConfig{job: cfg}
		endpoint      = "state2"
		haveInstances = map[string]agentState{
			"state2": agentState{
				instances: map[string]agent.ContainerInstance{
					id1: agent.ContainerInstance{},
					id2: agent.ContainerInstance{},
				},
			},
		}

		tr = transformer{
			ttl: time.Second * 10,
			unscheduled: map[string]pendingTask{
				id1: pendingTask{id: id1, endpoint: endpoint, cfg: agent.ContainerConfig{}, expiration: time.Now().Add(time.Second)},
				id2: pendingTask{id: id2, endpoint: endpoint, cfg: agent.ContainerConfig{}, expiration: time.Now().Add(time.Second)},
			},
			scheduled: map[string]pendingTask{},
		}
	)

	target := newMockTaskScheduler()
	tr.transform(wantJobs, haveInstances, target)
	if err := validate(target, tr, 0, 2, 0, 0); err != nil {
		t.Error(err)
	}

	target = newMockTaskScheduler()
	tr.transform(map[string]configstore.JobConfig{}, haveInstances, target)
	if err := validate(target, tr, 0, 2, 0, 0); err != nil {
		t.Error(err)
	}

	target = newMockTaskScheduler()
	tr.transform(
		wantJobs,
		map[string]agentState{
			endpoint: agentState{
				instances: map[string]agent.ContainerInstance{},
			},
		},
		target,
	)

	if err := validate(target, tr, 2, 0, 1, 0); err != nil {
		t.Error(err)
	}

	instances, ok := target.started[endpoint]
	if !ok {
		t.Fatalf("incorrect scheduling: %v", target.started)
	}

	if want, have := 2, len(instances); want != have {
		t.Errorf("incorrect number of scheduled intances: %d", len(instances))
	}

	if _, ok := instances[id1]; !ok {
		t.Errorf("config %q not correctly scheduled", id1)
	}

	if _, ok := instances[id2]; !ok {
		t.Errorf("config %q not correctly scheduled", id2)
	}

	tr.scheduled = map[string]pendingTask{}
	tr.unscheduled = map[string]pendingTask{
		id1: pendingTask{id: id1, endpoint: endpoint, cfg: agent.ContainerConfig{}, expiration: time.Now().Add(time.Second)},
		id2: pendingTask{id: id2, endpoint: endpoint, cfg: agent.ContainerConfig{}, expiration: time.Now().Add(time.Second)},
	}
	target = newMockTaskScheduler()
	tr.transform(
		map[string]configstore.JobConfig{},
		map[string]agentState{},
		target,
	)
	if err := validate(target, tr, 0, 0, 0, 0); err != nil {
		t.Error(err)
	}
}

func validate(target *mockTaskScheduler, tr transformer, wantSched, wantUnsched, wantStarted, wantStopped int) error {
	if want, have := wantSched, len(tr.scheduled); want != have {
		return fmt.Errorf("%d != %d", want, have)
	}

	if want, have := wantUnsched, len(tr.unscheduled); want != have {
		return fmt.Errorf("%d != %d", want, have)
	}

	if want, have := wantStarted, len(target.started); want != have {
		return fmt.Errorf("%d != %d", want, have)
	}

	if want, have := wantStopped, len(target.stopped); want != have {
		return fmt.Errorf("%d != %d", want, have)
	}

	return nil
}
