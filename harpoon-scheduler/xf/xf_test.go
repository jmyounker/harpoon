package xf

import (
	"io/ioutil"
	"log"
	"sync/atomic"
	"testing"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
	"github.com/soundcloud/harpoon/harpoon-scheduler/algo"
	"github.com/soundcloud/harpoon/harpoon-scheduler/xtime"
)

func TestRemoveDuplicates(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	jobConfig := configstore.JobConfig{
		Job:   "a",
		Scale: 1,
	}

	state := agent.StateEvent{
		Containers: map[string]agent.ContainerInstance{
			makeContainerID(jobConfig.Hash(), 0): agent.ContainerInstance{ContainerStatus: agent.ContainerStatusRunning},
		},
	}

	want := map[string]configstore.JobConfig{
		"a": jobConfig,
	}

	have := map[string]agent.StateEvent{
		"agent-one": state,
		"agent-two": state,
	}

	target := &mockTaskScheduler{}

	transform(want, have, target, map[string]algo.PendingTask{})

	if want, have := int32(0), atomic.LoadInt32(&target.schedules); want != have {
		t.Errorf("want %d schedule(s), have %d", want, have)
	}

	if want, have := int32(1), atomic.LoadInt32(&target.unschedules); want != have {
		t.Errorf("want %d unschedule(s), have %d", want, have)
	}
}

func TestRespectPending(t *testing.T) {
	Debugf = t.Logf

	jobConfig := configstore.JobConfig{
		Job:   "a",
		Scale: 2,
	}

	want := map[string]configstore.JobConfig{
		"a": jobConfig,
	}

	have := map[string]agent.StateEvent{
		"agent-one": agent.StateEvent{
			Containers: map[string]agent.ContainerInstance{
				makeContainerID(jobConfig.Hash(), 0): agent.ContainerInstance{ContainerStatus: agent.ContainerStatusRunning},
			},
		},
		"agent-two": agent.StateEvent{
			Containers: map[string]agent.ContainerInstance{
				makeContainerID(jobConfig.Hash(), 1): agent.ContainerInstance{ContainerStatus: agent.ContainerStatusCreated}, // starting...
			},
		},
	}

	target := &mockTaskScheduler{}

	pending := map[string]algo.PendingTask{
		makeContainerID(jobConfig.Hash(), 1): algo.PendingTask{Schedule: true, Deadline: xtime.Now().Add(10 * time.Second)},
	}

	pending = transform(want, have, target, pending)

	if want, have := int32(0), atomic.LoadInt32(&target.schedules); want != have {
		t.Errorf("want %d schedule(s), have %d", want, have)
	}

	if want, have := int32(0), atomic.LoadInt32(&target.unschedules); want != have {
		t.Errorf("want %d unschedule(s), have %d", want, have)
	}
}

func TestRespectResourceReservedForPendingTasks(t *testing.T) {
	jobConfig := configstore.JobConfig{
		Job:             "a",
		Scale:           2,
		ContainerConfig: agent.ContainerConfig{Resources: agent.Resources{Mem: 512}},
	}

	want := map[string]configstore.JobConfig{
		"a": jobConfig,
	}

	have := map[string]agent.StateEvent{
		"agent-one": agent.StateEvent{
			Containers: map[string]agent.ContainerInstance{},
			Resources: agent.HostResources{
				Mem:     agent.TotalReservedInt{Total: 1024, Reserved: 512},          // 1GB total, 512MB reserved
				CPU:     agent.TotalReserved{Total: 4.0, Reserved: 3.0},              // 4 CPU total, 3 CPU reserved
				Storage: agent.TotalReserved{Total: 100 * 1e10, Reserved: 70 * 1e10}, // 100GB total, 70GB reserved
				Volumes: []string{"/data/shared", "/data/wimpy"},
			},
		},
	}

	target := &mockTaskScheduler{}

	pending := map[string]algo.PendingTask{}
	pending = transform(want, have, target, pending)

	if want, have := int32(1), atomic.LoadInt32(&target.schedules); want != have {
		t.Errorf("want %d schedule(s), have %d", want, have)
	}

	if want, have := int32(0), atomic.LoadInt32(&target.unschedules); want != have {
		t.Errorf("want %d unschedule(s), have %d", want, have)
	}

	if want, have := 1, len(pending); want != have {
		t.Errorf("want %d pending tasks, have %d", want, have)
	}

	if have["agent-one"].Resources.Mem.Reserved != 512 {
		t.Errorf("host resources should not be changed")
	}

	// try second time to schedule this time with pending task
	target = &mockTaskScheduler{}
	pending = transform(want, have, target, pending)

	if want, have := int32(0), atomic.LoadInt32(&target.schedules); want != have {
		t.Errorf("want %d schedule(s), have %d", want, have)
	}

	if want, have := int32(0), atomic.LoadInt32(&target.unschedules); want != have {
		t.Errorf("want %d unschedule(s), have %d", want, have)
	}

	if want, have := 1, len(pending); want != have {
		t.Errorf("want %d pending tasks, have %d", want, have)
	}
}

func TestPendingExpiration(t *testing.T) {
	Debugf = t.Logf

	jobConfig := configstore.JobConfig{
		Job:   "a",
		Scale: 2,
	}

	want := map[string]configstore.JobConfig{
		"a": jobConfig,
	}

	have := map[string]agent.StateEvent{
		"agent-one": agent.StateEvent{
			Containers: map[string]agent.ContainerInstance{
				makeContainerID(jobConfig.Hash(), 0): agent.ContainerInstance{ContainerStatus: agent.ContainerStatusRunning},
			},
		},
		"agent-two": agent.StateEvent{
			Containers: map[string]agent.ContainerInstance{},
		},
	}

	fakeNow := time.Now()
	xtime.Now = func() time.Time { return fakeNow }

	target := &mockTaskScheduler{}

	deadline := fakeNow.Add(time.Second)
	pending := map[string]algo.PendingTask{
		makeContainerID(jobConfig.Hash(), 1): algo.PendingTask{Schedule: true, Deadline: deadline},
	}

	// The first transform should detect the container as pending, and not
	// issue any mutations.

	pending = transform(want, have, target, pending)

	if want, have := 1, len(pending); want != have {
		t.Errorf("want %d pending, have %d", want, have)
	}

	if want, have := int32(0), atomic.LoadInt32(&target.schedules); want != have {
		t.Errorf("want %d schedule(s), have %d", want, have)
	}

	if want, have := int32(0), atomic.LoadInt32(&target.unschedules); want != have {
		t.Errorf("want %d unschedule(s), have %d", want, have)
	}

	// Advance past our pendingTask deadline.

	fakeNow = deadline.Add(time.Millisecond)

	// The next transform should detect the pendingTask as expired and delete
	// it. And because the container is not present, it should emit a schedule
	// mutation. That has the side effect of re-adding it to the pending map
	// :)

	pending = transform(want, have, target, pending)

	if want, have := 1, len(pending); want != have {
		t.Errorf("want %d pending, have %d", want, have)
	}

	if want, have := int32(1), atomic.LoadInt32(&target.schedules); want != have {
		t.Errorf("want %d schedule(s), have %d", want, have)
	}

	if want, have := int32(0), atomic.LoadInt32(&target.unschedules); want != have {
		t.Errorf("want %d unschedule(s), have %d", want, have)
	}
}

func TestRetryingMutation(t *testing.T) {
	Debugf = t.Logf

	jobConfig := configstore.JobConfig{
		Job:   "a",
		Scale: 1,
	}

	state := agent.StateEvent{
		Containers: map[string]agent.ContainerInstance{
			makeContainerID(jobConfig.Hash(), 0): agent.ContainerInstance{ContainerStatus: agent.ContainerStatusRunning},
		},
	}

	// want an empty state
	want := map[string]configstore.JobConfig{}

	// have one container running
	have := map[string]agent.StateEvent{"the-agent": state}

	target := &mockTaskScheduler{}

	fakeNow := time.Now()
	xtime.Now = func() time.Time { return fakeNow }

	// the container has a pending unschedule mutation
	pending := map[string]algo.PendingTask{
		makeContainerID(jobConfig.Hash(), 0): algo.PendingTask{
			Schedule: false,
			Deadline: fakeNow.Add(time.Second),
			Endpoint: "the-agent",
		},
	}

	// In the first transform, we have a running container that's ostensibly
	// pending-unschedule. The pending map should be unchanged.

	pending = transform(want, have, target, pending)

	if want, have := 1, len(pending); want != have {
		t.Errorf("want %d pending, have %d", want, have)
	}

	if want, have := int32(0), atomic.LoadInt32(&target.schedules); want != have {
		t.Errorf("want %d schedule(s), have %d", want, have)
	}

	if want, have := int32(0), atomic.LoadInt32(&target.unschedules); want != have {
		t.Errorf("want %d unschedule(s), have %d", want, have)
	}

	// Advance our time past the deadline, and try again. We should delete the
	// original pending task, emit an unschedule mutation, and add a new
	// pending task.

	fakeNow = fakeNow.Add(Tolerance + time.Millisecond)
	pending = transform(want, have, target, pending)

	if want, have := 1, len(pending); want != have {
		t.Errorf("want %d pending, have %d", want, have)
	}

	if want, have := int32(0), atomic.LoadInt32(&target.schedules); want != have {
		t.Errorf("want %d schedule(s), have %d", want, have)
	}

	if want, have := int32(1), atomic.LoadInt32(&target.unschedules); want != have {
		t.Errorf("want %d unschedule(s), have %d", want, have)
	}

	// Do it again. Since the "have" map isn't changing, we should observe the
	// same effect.

	fakeNow = fakeNow.Add(Tolerance + time.Millisecond)
	pending = transform(want, have, target, pending)

	if want, have := 1, len(pending); want != have {
		t.Errorf("want %d pending, have %d", want, have)
	}

	if want, have := int32(0), atomic.LoadInt32(&target.schedules); want != have {
		t.Errorf("want %d schedule(s), have %d", want, have)
	}

	if want, have := int32(2), atomic.LoadInt32(&target.unschedules); want != have {
		t.Errorf("want %d unschedule(s), have %d", want, have)
	}

	// Unschedule the container manually, and do another transform. We should
	// remove the entry from the pending map, and issue no further mutations.

	have["the-agent"] = agent.StateEvent{} // no containers

	fakeNow = fakeNow.Add(Tolerance + time.Millisecond)
	pending = transform(want, have, target, pending)

	if want, have := 0, len(pending); want != have {
		t.Errorf("want %d pending, have %d", want, have)
	}

	if want, have := int32(0), atomic.LoadInt32(&target.schedules); want != have {
		t.Errorf("want %d schedule(s), have %d", want, have)
	}

	if want, have := int32(2), atomic.LoadInt32(&target.unschedules); want != have {
		t.Errorf("want %d unschedule(s), have %d", want, have)
	}
}

type mockTaskScheduler struct {
	schedules   int32
	unschedules int32
}

func (s *mockTaskScheduler) Schedule(endpoint, id string, _ agent.ContainerConfig) error {
	atomic.AddInt32(&s.schedules, 1)
	return nil
}

func (s *mockTaskScheduler) Unschedule(endpoint, id string) error {
	atomic.AddInt32(&s.unschedules, 1)
	return nil
}
