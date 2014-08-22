package main

import (
	"io/ioutil"
	"log"
	"testing"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

func TestScheduleJob(t *testing.T) {
	log.SetOutput(ioutil.Discard)
	//log.SetFlags(log.Lmicroseconds)

	var (
		target  = newMockTaskScheduler()
		current = map[string]map[string]agent.ContainerInstance{"http://københavn.guru:1813": {}}
		job     = "kierkegaard"
		scale   = 3
		want    = map[string]int{job: scale}
		cfg     = configstore.JobConfig{Job: job, Scale: scale}
	)

	if err := scheduleJob(cfg, current, target); err != nil {
		t.Fatal(err)
	}

	for _, spec := range target.started {
		//t.Logf("%s on %s", spec.ContainerID, spec.Endpoint)
		want[spec.Job] = want[spec.Job] - 1
	}

	for job, zero := range want {
		if zero != 0 {
			t.Errorf("%s: offset of %d", job, zero)
		}

		delete(want, job)
	}

	if len(want) > 0 {
		t.Errorf("%d unknown jobs: %v", len(want), want)
	}
}

func TestUnscheduleJob(t *testing.T) {
	log.SetOutput(ioutil.Discard)
	//log.SetFlags(log.Lmicroseconds)

	var (
		aquinas  = configstore.JobConfig{Job: "aquinas", Scale: 2}
		berkeley = configstore.JobConfig{Job: "berkeley", Scale: 3}
		jobs     = []configstore.JobConfig{aquinas, berkeley}
		target   = simpleMockTaskScheduler{}
	)

	// Schedule both jobs

	for _, cfg := range jobs {
		for i := 0; i < cfg.Scale; i++ {
			spec := taskSpec{ContainerID: makeContainerID(cfg, i)}
			if err := target.schedule(spec); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Unschedule one job

	if err := unscheduleJob(aquinas, target.current(), target); err != nil {
		t.Fatal(err)
	}

	// Verify counts

	if want, have := map[string]int{
		makeContainerID(aquinas, 0):  0,
		makeContainerID(aquinas, 1):  0,
		makeContainerID(berkeley, 0): 1,
		makeContainerID(berkeley, 1): 1,
		makeContainerID(berkeley, 2): 1,
	}, target; !deepEqualIgnoreOrder(want, have) {
		t.Fatalf("want %#+v, have %#+v", want, have)
	}
}

func TestMigrateJob(t *testing.T) {
	t.Skip("not yet implemented")
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
