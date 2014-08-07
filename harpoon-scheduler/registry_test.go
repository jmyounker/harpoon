package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestRegistrySaveLoad(t *testing.T) {
	log.SetOutput(ioutil.Discard)
	//log.SetFlags(log.Lmicroseconds)

	var (
		testFilename    = "test_registry_save_load.json"
		testContainerID = "hulk-hogan"
		testTaskSpec    = taskSpec{Endpoint: "https://super.cool:1337"}
	)

	defer os.Remove(testFilename)

	registry, err := newRegistry(nil, testFilename)
	if err != nil {
		t.Fatal(err)
	}

	c := make(chan schedulingSignalWithContext)
	if err := registry.schedule(testContainerID, testTaskSpec, c); err != nil {
		t.Fatal(err)
	}

	received := make(chan schedulingSignalWithContext)
	go func() { received <- <-c }() // buffer of 1

	registry.signal(testContainerID, signalScheduleSuccessful)

	select {
	case <-received:
	case <-time.After(time.Millisecond):
		t.Fatal("never got signal after a successful schedule")
	}

	buf, err := ioutil.ReadFile(testFilename)
	if err != nil {
		t.Fatal(err)
	}

	var have map[string]taskSpec
	if err := json.Unmarshal(buf, &have); err != nil {
		t.Fatal(err)
	}

	if want := registry.scheduled; !reflect.DeepEqual(have, want) {
		t.Fatalf("have \n\t%#+v, want \n\t%#+v", have, want)
	}

	registry2, err := newRegistry(nil, testFilename)
	if err != nil {
		t.Fatal(err)
	}

	if have, want := registry2.scheduled, registry.scheduled; !reflect.DeepEqual(have, want) {
		t.Fatalf("have \n\t%#+v, want \n\t%#+v", have, want)
	}
}

func TestRegistrySchedule(t *testing.T) {
	log.SetOutput(ioutil.Discard)
	//log.SetFlags(log.Lmicroseconds)

	var (
		registry        = newTestRegistry(t)
		testContainerID = "test-container-id"
		testTaskSpec    = taskSpec{Endpoint: "http://nonexistent.berlin:1234"}
	)

	// Try a bad container ID.
	if err := registry.schedule("", testTaskSpec, nil); err == nil {
		t.Errorf("while scheduling bad container ID: expected error, got none")
	}

	// Good path.
	c := make(chan schedulingSignalWithContext)
	if err := registry.schedule(testContainerID, testTaskSpec, c); err != nil {
		t.Errorf("while scheduling good container: %s", err)
	}
	if _, ok := registry.pendingSchedule[testContainerID]; !ok {
		t.Fatalf("%s isn't pending-schedule", testContainerID)
	}

	// Try to double-schedule.
	if err := registry.schedule(testContainerID, testTaskSpec, nil); err == nil {
		t.Errorf("while scheduling a container that's already pending-schedule: expected error, got none")
	}

	// Pretend we're a transformer, and move the thing to scheduled.
	received := make(chan schedulingSignalWithContext) // make an intermediary chan, so
	go func() { received <- <-c }()                    // we don't block the signaler

	registry.signal(testContainerID, signalScheduleSuccessful)

	select {
	case <-received:
	case <-time.After(time.Millisecond):
		t.Fatal("never got signal after a successful schedule")
	}

	if _, ok := registry.pendingSchedule[testContainerID]; ok {
		t.Fatalf("%s is still pending-schedule", testContainerID)
	}

	if _, ok := registry.scheduled[testContainerID]; !ok {
		t.Fatalf("%s isn't scheduled", testContainerID)
	}

	// Try to schedule it again.
	if err := registry.schedule(testContainerID, testTaskSpec, nil); err == nil {
		t.Errorf("while scheduling a container that's already scheduled: expected error, got none")
	}

	// Move it to pending-unschedule.
	if err := registry.unschedule(testContainerID, testTaskSpec, nil); err != nil {
		t.Fatalf("while unscheduling: %s", err)
	}

	// Try to schedule it again.
	if err := registry.schedule(testContainerID, testTaskSpec, nil); err == nil {
		t.Errorf("while scheduling a container that's already pending-unschedule: expected error, got none")
	}
}

func TestRegistryUnschedule(t *testing.T) {
	log.SetOutput(ioutil.Discard)
	//log.SetFlags(log.Lmicroseconds)

	var (
		registry        = newTestRegistry(t)
		testContainerID = "test-container-id"
		testTaskSpec    = taskSpec{Endpoint: "http://nonexistent.berlin:1234"}
	)

	// Try a bad container ID.
	if err := registry.unschedule("", testTaskSpec, nil); err == nil {
		t.Errorf("while unscheduling bad container ID: expected error, got none")
	} else {
		t.Logf("unscheduling a bad container ID: %s (good)", err)
	}

	// Try a good but non-present container ID.
	if err := registry.unschedule(testContainerID, testTaskSpec, nil); err == nil {
		t.Errorf("while unscheduling an unknown container: expected error, got none")
	} else {
		t.Logf("unscheduling an unknown container: %s (good)", err)
	}

	// Make something pending-schedule.
	if err := registry.schedule(testContainerID, testTaskSpec, nil); err != nil {
		t.Fatalf("while scheduling good container: %s", err)
	}
	if _, ok := registry.pendingSchedule[testContainerID]; !ok {
		t.Fatalf("%s isn't pending-schedule", testContainerID)
	}

	// Try to unschedule it.
	if err := registry.unschedule(testContainerID, testTaskSpec, nil); err == nil {
		t.Errorf("while unscheduling a pending-schedule container: expected error, got none")
	} else {
		t.Logf("unscheduling a pending-schedule container: %s (good)", err)
	}

	// Pretend we're a transformer, and move it to scheduled.
	registry.signal(testContainerID, signalScheduleSuccessful)

	if _, ok := registry.scheduled[testContainerID]; !ok {
		t.Fatalf("%s isn't scheduled", testContainerID)
	}

	// Try to unschedule it.
	c := make(chan schedulingSignalWithContext)
	if err := registry.unschedule(testContainerID, testTaskSpec, c); err != nil {
		t.Errorf("while unscheduling a scheduled container: %s", err)
	}
	if _, ok := registry.pendingUnschedule[testContainerID]; !ok {
		t.Fatalf("%s isn't pending-unschedule", testContainerID)
	}

	// Try to unschedule it again.
	if err := registry.unschedule(testContainerID, testTaskSpec, nil); err == nil {
		t.Errorf("while unscheduling an pending-unschedule container: expected error, got none")
	} else {
		t.Logf("unscheduling a pending-unschedule container: %s (good)", err)
	}

	// Pretend we're a transformer, and move the thing to deleted.
	received := make(chan schedulingSignalWithContext) // make an intermediary chan, so
	go func() { received <- <-c }()                    // we don't block the signaler

	registry.signal(testContainerID, signalUnscheduleSuccessful)

	select {
	case <-received:
	case <-time.After(time.Millisecond):
		t.Fatal("never got signal after a successful unschedule")
	}

	if _, ok := registry.pendingUnschedule[testContainerID]; ok {
		t.Fatalf("%s is still pending-unschedule", testContainerID)
	}
}

func newTestRegistry(t *testing.T) *registry {
	registry, err := newRegistry(nil, "")
	if err != nil {
		t.Fatal(err)
	}
	return registry
}
