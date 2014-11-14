package xf

import (
	"io/ioutil"
	"log"
	"sync"
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
				Mem:     agent.TotalReservedInt{Total: 1024, Reserved: 512},             // 1GB total, 512MB reserved
				CPU:     agent.TotalReserved{Total: 4.0, Reserved: 3.0},                 // 4 CPU total, 3 CPU reserved
				Storage: agent.TotalReservedInt{Total: 100 * 1e10, Reserved: 70 * 1e10}, // 100GB total, 70GB reserved
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

// https://github.com/soundcloud/harpoon/issues/121
func TestSchedulerRestartsDisappearingTasksIndefinitely(t *testing.T) {
	Debugf = t.Logf

	var (
		desire = newFakeDesireBroadcaster()
		actual = newFakeActualBroadcaster()
		target = &mockTaskScheduler{}
	)

	// Start a scheduler (Transform) talking to 2 agents

	actual.set(map[string]agent.StateEvent{
		"leeroy":  agent.StateEvent{},
		"jenkins": agent.StateEvent{},
	})

	go Transform(desire, actual, target)
	time.Sleep(time.Millisecond) // let Transform subscribe

	// Schedule a job with scale = 3

	jobConfig := configstore.JobConfig{Job: "a", Scale: 3}
	desire.set(map[string]configstore.JobConfig{"a": jobConfig})
	time.Sleep(time.Millisecond)

	if want, have := int32(3), target.schedules; want != have {
		t.Fatalf("want %d, have %d", want, have)
	}

	var (
		id0 = makeContainerID(jobConfig.Hash(), 0)
		ci0 = agent.ContainerInstance{ID: id0, ContainerStatus: agent.ContainerStatusRunning}
		id1 = makeContainerID(jobConfig.Hash(), 1)
		ci1 = agent.ContainerInstance{ID: id1, ContainerStatus: agent.ContainerStatusRunning}
		id2 = makeContainerID(jobConfig.Hash(), 2)
		ci2 = agent.ContainerInstance{ID: id2, ContainerStatus: agent.ContainerStatusRunning}
	)

	actual.set(map[string]agent.StateEvent{
		"leeroy":  agent.StateEvent{Containers: map[string]agent.ContainerInstance{id0: ci0, id1: ci1}},
		"jenkins": agent.StateEvent{Containers: map[string]agent.ContainerInstance{id2: ci2}},
	})
	t.Logf("(all tasks started and running)")
	time.Sleep(time.Millisecond)

	// Disappear one of the tasks

	actual.set(map[string]agent.StateEvent{
		"leeroy":  agent.StateEvent{Containers: map[string]agent.ContainerInstance{id0: ci0}},
		"jenkins": agent.StateEvent{Containers: map[string]agent.ContainerInstance{id2: ci2}},
	})
	t.Logf("(task 1 has disappeared)")
	time.Sleep(time.Millisecond)

	// The scheduler (Transform) should restart it

	if want, have := int32(4), target.schedules; want != have {
		t.Fatalf("want %d, have %d", want, have)
	}

	actual.set(map[string]agent.StateEvent{
		"leeroy":  agent.StateEvent{Containers: map[string]agent.ContainerInstance{id0: ci0, id1: ci1}},
		"jenkins": agent.StateEvent{Containers: map[string]agent.ContainerInstance{id2: ci2}},
	})
	t.Logf("(the task comes back)")
	time.Sleep(time.Millisecond)

	// Disappear the task again

	actual.set(map[string]agent.StateEvent{
		"leeroy":  agent.StateEvent{Containers: map[string]agent.ContainerInstance{id0: ci0}},
		"jenkins": agent.StateEvent{Containers: map[string]agent.ContainerInstance{id2: ci2}},
	})
	t.Logf("(task 1 has disappeared again)")
	time.Sleep(time.Millisecond)

	// The scheduler (Transform) should restart it again

	if want, have := int32(5), target.schedules; want != have {
		t.Fatalf("want %d, have %d", want, have)
	}

	actual.set(map[string]agent.StateEvent{
		"leeroy":  agent.StateEvent{Containers: map[string]agent.ContainerInstance{id0: ci0, id1: ci1}},
		"jenkins": agent.StateEvent{Containers: map[string]agent.ContainerInstance{id2: ci2}},
	})
	t.Logf("(the task comes back again)")
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

type fakeDesireBroadcaster struct {
	sync.RWMutex
	subs map[chan<- map[string]configstore.JobConfig]struct{}
	m    map[string]configstore.JobConfig
}

func newFakeDesireBroadcaster() *fakeDesireBroadcaster {
	return &fakeDesireBroadcaster{
		subs: map[chan<- map[string]configstore.JobConfig]struct{}{},
		m:    map[string]configstore.JobConfig{},
	}
}

func (b *fakeDesireBroadcaster) Subscribe(c chan<- map[string]configstore.JobConfig) {
	b.Lock()
	defer b.Unlock()

	b.subs[c] = struct{}{}
	go func(m map[string]configstore.JobConfig) { c <- m }(b.m)
}

func (b *fakeDesireBroadcaster) Unsubscribe(c chan<- map[string]configstore.JobConfig) {
	b.Lock()
	defer b.Unlock()

	delete(b.subs, c)
	close(c)
}

func (b *fakeDesireBroadcaster) set(m map[string]configstore.JobConfig) {
	b.Lock()
	defer b.Unlock()

	b.m = m
	b.broadcast()
}

func (b *fakeDesireBroadcaster) broadcast() {
	log.Printf("### fakeDesireBroadcaster broadcasting to %d", len(b.subs))
	for c := range b.subs {
		select {
		case c <- b.m:
		case <-time.After(time.Millisecond):
			panic("slow subscriber in fakeDesireBroadcaster")
		}
	}
}

type fakeActualBroadcaster struct {
	sync.RWMutex
	subs map[chan<- map[string]agent.StateEvent]struct{}
	m    map[string]agent.StateEvent
}

func newFakeActualBroadcaster() *fakeActualBroadcaster {
	return &fakeActualBroadcaster{
		subs: map[chan<- map[string]agent.StateEvent]struct{}{},
		m:    map[string]agent.StateEvent{},
	}
}

func (b *fakeActualBroadcaster) Subscribe(c chan<- map[string]agent.StateEvent) {
	b.Lock()
	defer b.Unlock()

	b.subs[c] = struct{}{}
	go func(m map[string]agent.StateEvent) { c <- m }(b.m)
}

func (b *fakeActualBroadcaster) Unsubscribe(c chan<- map[string]agent.StateEvent) {
	b.Lock()
	defer b.Unlock()

	delete(b.subs, c)
	close(c)
}

func (b *fakeActualBroadcaster) set(m map[string]agent.StateEvent) {
	b.Lock()
	defer b.Unlock()

	b.m = m
	b.broadcast()
}

func (b *fakeActualBroadcaster) broadcast() {
	for c := range b.subs {
		select {
		case c <- b.m:
		case <-time.After(time.Millisecond):
			panic("slow subscriber in fakeActualBroadcaster")
		}
	}
}
