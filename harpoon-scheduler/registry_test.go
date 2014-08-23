package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

func TestRegistryStartStop(t *testing.T) {
	var (
		registry = newRealRegistry("")
		updatec  = make(chan map[string]configstore.JobConfig)
		requestc = make(chan map[string]configstore.JobConfig)
	)

	defer registry.quit()

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

	registry.subscribe(updatec)
	defer close(updatec)
	defer registry.unsubscribe(updatec)

	var (
		job1 = configstore.JobConfig{Job: "table"}
		job2 = configstore.JobConfig{Job: "chair"}
	)

	if err := registry.schedule(job1); err != nil {
		t.Fatal(err)
	}

	if want, have := map[string]configstore.JobConfig{
		job1.Hash(): job1,
	}, <-requestc; !reflect.DeepEqual(want, have) {
		t.Fatalf("want %v, have %v", want, have)
	}

	if err := registry.schedule(job2); err != nil {
		t.Fatal(err)
	}

	if want, have := map[string]configstore.JobConfig{
		job1.Hash(): job1,
		job2.Hash(): job2,
	}, <-requestc; !reflect.DeepEqual(want, have) {
		t.Fatalf("want %v, have %v", want, have)
	}

	if err := registry.unschedule(job1); err != nil {
		t.Fatal(err)
	}

	if want, have := map[string]configstore.JobConfig{
		job2.Hash(): job2,
	}, <-requestc; !reflect.DeepEqual(want, have) {
		t.Fatalf("want %v, have %v", want, have)
	}

	if err := registry.unschedule(job2); err != nil {
		t.Fatal(err)
	}

	if want, have := map[string]configstore.JobConfig{}, <-requestc; !reflect.DeepEqual(want, have) {
		t.Fatalf("want %v, have %v", want, have)
	}
}

func TestRegistrySaveLoad(t *testing.T) {
	var (
		filename  = "registry-test-save-load.json"
		registry1 = newRealRegistry(filename)
		job       = configstore.JobConfig{Job: "π"}
	)

	defer os.Remove(filename)

	defer registry1.quit()

	// Schedule a thing.

	if err := registry1.schedule(job); err != nil {
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
	registry1.subscribe(check1c)
	defer registry1.unsubscribe(check1c)

	if want, have := <-check1c, fromDisk; !reflect.DeepEqual(want, have) {
		t.Fatalf("want %v, have %v", want, have)
	}

	// Boot up another registry on top of the same file. Just for the test.
	// You'd never do this in real life. Race conditions everywhere.

	registry2 := newRealRegistry(filename)
	defer registry2.quit()

	// Verify it loaded the previously-persisted state.

	check2c := make(chan map[string]configstore.JobConfig)
	registry2.subscribe(check2c)
	defer registry2.unsubscribe(check2c)

	if want, have := fromDisk, <-check2c; !reflect.DeepEqual(want, have) {
		t.Fatalf("want %v, have %v", want, have)
	}
}
