package main

import (
	"io/ioutil"
	"log"
	"reflect"
	"testing"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

func TestTransform(t *testing.T) {
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
	}

	for i, input := range []testCase{
		{
			wantJobs: map[string]configstore.JobConfig{job: cfg},
			haveInstances: map[string]agentState{
				endpoint: agentState{
					instances: map[string]agent.ContainerInstance{},
				},
			},
			started: map[string]map[string]agent.ContainerConfig{endpoint: map[string]agent.ContainerConfig{id: agent.ContainerConfig{}}},
			stopped: map[string]map[string]struct{}{},
		},
		{
			wantJobs: map[string]configstore.JobConfig{job: cfg},
			haveInstances: map[string]agentState{
				endpoint: agentState{
					instances: map[string]agent.ContainerInstance{id: agent.ContainerInstance{}},
				},
			},
			started: map[string]map[string]agent.ContainerConfig{},
			stopped: map[string]map[string]struct{}{},
		},
		{
			wantJobs: map[string]configstore.JobConfig{},
			haveInstances: map[string]agentState{
				endpoint: agentState{
					instances: map[string]agent.ContainerInstance{job: agent.ContainerInstance{}},
				},
			},
			started: map[string]map[string]agent.ContainerConfig{},
			stopped: map[string]map[string]struct{}{endpoint: map[string]struct{}{job: struct{}{}}},
		},
	} {
		target := newMockTaskScheduler()

		transform(input.wantJobs, input.haveInstances, target)

		if want, have := input.started, target.started; !reflect.DeepEqual(want, have) {
			t.Errorf("%d: started: want %v, have %v", i, want, have)
		}

		if want, have := input.stopped, target.stopped; !reflect.DeepEqual(want, have) {
			t.Errorf("%d: stopped: want %v, have %v", i, want, have)
		}
	}
}
