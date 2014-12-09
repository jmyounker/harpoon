package registry_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
	"github.com/soundcloud/harpoon/harpoon-scheduler/registry"
)

func TestRegistryStartStop(t *testing.T) {
	var (
		registry = registry.New("")
		updatec  = make(chan map[string]configstore.JobConfig)
		requestc = make(chan map[string]configstore.JobConfig)
	)

	defer registry.Quit()

	go func() {
		scheduled, ok := <-updatec
		if !ok {
			return
		}

		for {
			select {
			case requestc <- scheduled:
			case scheduled, ok = <-updatec:
				if !ok {
					return
				}
			}
		}
	}()

	registry.Subscribe(updatec)
	defer close(updatec)
	defer registry.Unsubscribe(updatec)

	var (
		job1 = configstore.JobConfig{ContainerConfig: agent.ContainerConfig{Job: "table"}}
		job2 = configstore.JobConfig{ContainerConfig: agent.ContainerConfig{Job: "chair"}}
	)

	if err := registry.Schedule(job1); err != nil {
		t.Fatal(err)
	}

	if want, have := map[string]configstore.JobConfig{
		job1.Hash(): job1,
	}, <-requestc; !reflect.DeepEqual(want, have) {
		t.Fatalf("want %v, have %v", want, have)
	}

	if err := registry.Schedule(job2); err != nil {
		t.Fatal(err)
	}

	if want, have := map[string]configstore.JobConfig{
		job1.Hash(): job1,
		job2.Hash(): job2,
	}, <-requestc; !reflect.DeepEqual(want, have) {
		t.Fatalf("want %v, have %v", want, have)
	}

	if err := registry.Unschedule(job1.Hash()); err != nil {
		t.Fatal(err)
	}

	if want, have := map[string]configstore.JobConfig{
		job2.Hash(): job2,
	}, <-requestc; !reflect.DeepEqual(want, have) {
		t.Fatalf("want %v, have %v", want, have)
	}

	if err := registry.Unschedule(job2.Hash()); err != nil {
		t.Fatal(err)
	}

	if want, have := map[string]configstore.JobConfig{}, <-requestc; !reflect.DeepEqual(want, have) {
		t.Fatalf("want %v, have %v", want, have)
	}
}

func TestRegistrySaveLoad(t *testing.T) {
	var (
		filename  = "registry-test-save-load.json"
		registry1 = registry.New(filename)
		job       = configstore.JobConfig{ContainerConfig: agent.ContainerConfig{Job: "π"}}
	)

	defer os.Remove(filename)

	defer registry1.Quit()

	// Schedule a thing.

	if err := registry1.Schedule(job); err != nil {
		t.Fatal(err)
	}

	// Verify it persisted.

	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	var fromDisk map[string]configstore.JobConfig
	if err := json.Unmarshal(buf, &fromDisk); err != nil {
		t.Fatal(err)
	}

	check1c := make(chan map[string]configstore.JobConfig)
	registry1.Subscribe(check1c)
	defer registry1.Unsubscribe(check1c)

	if want, have := <-check1c, fromDisk; !reflect.DeepEqual(want, have) {
		t.Fatalf("want %v, have %v", want, have)
	}

	// Boot up another registry on top of the same file. Just for the test.
	// You'd never do this in real life. Race conditions everywhere.

	registry2 := registry.New(filename)
	defer registry2.Quit()

	// Verify it loaded the previously-persisted state.

	check2c := make(chan map[string]configstore.JobConfig)
	registry2.Subscribe(check2c)
	defer registry2.Unsubscribe(check2c)

	if want, have := fromDisk, <-check2c; !reflect.DeepEqual(want, have) {
		t.Fatalf("want %v, have %v", want, have)
	}
}
