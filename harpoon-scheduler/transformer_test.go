package main

import (
	"reflect"
	"testing"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestTransform(t *testing.T) {
	var (
		endpoint1 = "http://berlin.info:1234"
		id1       = "professor-wiggles"
		taskSpec1 = taskSpec{Endpoint: endpoint1, ContainerID: id1}
	)

	type testCase struct {
		want    map[string]map[string]taskSpec
		have    map[string]map[string]agent.ContainerInstance
		started []taskSpec
		stopped []endpointID
	}

	for i, input := range []testCase{
		{
			want:    map[string]map[string]taskSpec{endpoint1: map[string]taskSpec{id1: taskSpec1}},
			have:    map[string]map[string]agent.ContainerInstance{},
			started: []taskSpec{taskSpec1},
			stopped: []endpointID{},
		},
		{
			want:    map[string]map[string]taskSpec{endpoint1: map[string]taskSpec{id1: taskSpec1}},
			have:    map[string]map[string]agent.ContainerInstance{endpoint1: map[string]agent.ContainerInstance{id1: agent.ContainerInstance{}}},
			started: []taskSpec{},
			stopped: []endpointID{},
		},
		{
			want:    map[string]map[string]taskSpec{},
			have:    map[string]map[string]agent.ContainerInstance{endpoint1: map[string]agent.ContainerInstance{id1: agent.ContainerInstance{}}},
			started: []taskSpec{},
			stopped: []endpointID{{endpoint1, id1}},
		},
	} {
		target := newMockTaskScheduler()

		transform(input.want, input.have, target)

		if want, have := input.started, target.started; !reflect.DeepEqual(want, have) {
			t.Errorf("%d: started: want %v, have %v", i, want, have)
		}

		if want, have := input.stopped, target.stopped; !reflect.DeepEqual(want, have) {
			t.Errorf("%d: stopped: want %v, have %v", i, want, have)
		}
	}
}
