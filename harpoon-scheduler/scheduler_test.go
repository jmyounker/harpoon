package main

import (
	"testing"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

func TestScheduleJob(t *testing.T) {
	var (
		target    = newMockTaskScheduler()
		current   = map[string]map[string]agent.ContainerInstance{"http://dummy.agent.cool": {}}
		want      = map[string]int{}
		jobConfig = configstore.JobConfig{
			JobName: "armadillo",
			Tasks: []configstore.TaskConfig{
				{TaskName: "buffalo", Scale: 1},
				{TaskName: "chinchilla", Scale: 2},
			},
		}
	)

	if err := scheduleJob(jobConfig, current, target); err != nil {
		t.Fatal(err)
	}

	for _, taskConfig := range jobConfig.Tasks {
		want[taskConfig.TaskName] = taskConfig.Scale
	}

	for _, spec := range target.started {
		t.Logf("%s on %s", spec.ContainerID, spec.Endpoint)
		want[spec.TaskName] = want[spec.TaskName] - 1
	}

	for taskName, zero := range want {
		if zero != 0 {
			t.Errorf("%s: offset of %d", taskName, zero)
		}

		delete(want, taskName)
	}

	if len(want) > 0 {
		t.Errorf("%d unknown tasks: %v", len(want), want)
	}
}

func TestUnscheduleJob(t *testing.T) {
	var (
		adam    = configstore.TaskConfig{TaskName: "adam", Scale: 1}
		betty   = configstore.TaskConfig{TaskName: "betty", Scale: 2}
		charlie = configstore.JobConfig{JobName: "charlie", Tasks: []configstore.TaskConfig{adam, betty}}
		ygritte = configstore.TaskConfig{TaskName: "ygritte", Scale: 1}
		zachary = configstore.JobConfig{JobName: "zachary", Tasks: []configstore.TaskConfig{ygritte}}
		jobs    = []configstore.JobConfig{charlie, zachary}
		target  = simpleMockTaskScheduler{}
	)

	for _, jobConfig := range jobs {
		for _, taskConfig := range jobConfig.Tasks {
			for i := 0; i < taskConfig.Scale; i++ {
				if err := target.schedule(taskSpec{
					ContainerID: makeContainerID(jobConfig, taskConfig, i),
				}); err != nil {
					t.Fatal(err)
				}
			}
		}
	}

	if err := unscheduleJob(charlie, target.current(), target); err != nil {
		t.Fatal(err)
	}

	if want, have := map[string]int{
		makeContainerID(charlie, adam, 0):    0,
		makeContainerID(charlie, betty, 0):   0,
		makeContainerID(charlie, betty, 1):   0,
		makeContainerID(zachary, ygritte, 0): 1,
	}, target; !deepEqualIgnoreOrder(want, have) {
		t.Fatalf("want %#+v, have %#+v", want, have)
	}
}

func TestMigrateJob(t *testing.T) {
	t.Skip("TODO")
}

type simpleMockTaskScheduler map[string]int

var _ taskScheduler = simpleMockTaskScheduler{}

func (s simpleMockTaskScheduler) schedule(spec taskSpec) error {
	s[spec.ContainerID] = s[spec.ContainerID] + 1
	return nil
}

func (s simpleMockTaskScheduler) unschedule(_, containerID string) error {
	s[containerID] = s[containerID] - 1
	return nil
}

func (s simpleMockTaskScheduler) current() map[string]map[string]agent.ContainerInstance {
	var (
		ep  = "irrelevant-endpoint"
		out = map[string]map[string]agent.ContainerInstance{ep: {}}
	)

	for containerID := range s {
		out[ep][containerID] = agent.ContainerInstance{}
	}

	return out
}

func deepEqualIgnoreOrder(a, b map[string]int) bool {
	for k, v := range a {
		if v2, ok := b[k]; !ok || v != v2 {
			return false
		}
	}

	for k, v := range b {
		if v2, ok := a[k]; !ok || v != v2 {
			return false
		}
	}

	return true
}
