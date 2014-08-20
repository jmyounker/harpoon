package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"testing"
)

func TestRegistryStartStop(t *testing.T) {
	var (
		registry = newRealRegistry("")
		updatec  = make(chan map[string]taskSpec)
		requestc = make(chan map[string]taskSpec)
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
		endpoint1 = "http://marquadt.info:5001"
		endpoint2 = "http://tom-jenkinson.biz:9000"
		id1       = "das-ist-die"
		id2       = "wer-ist-das"
		spec1     = taskSpec{Endpoint: endpoint1, ContainerID: id1}
		spec2     = taskSpec{Endpoint: endpoint2, ContainerID: id2}
	)

	if err := registry.schedule(spec1); err != nil {
		t.Fatal(err)
	}

	if err := verifySpecs(t, <-requestc,
		endpoint1, id1,
	); err != nil {
		t.Fatal(err)
	}

	if err := registry.schedule(spec2); err != nil {
		t.Fatal(err)
	}

	if err := verifySpecs(t, <-requestc,
		endpoint1, id1,
		endpoint2, id2,
	); err != nil {
		t.Fatal(err)
	}

	if err := registry.unschedule(endpoint1, id1); err != nil {
		t.Fatal(err)
	}

	if err := verifySpecs(t, <-requestc,
		endpoint2, id2,
	); err != nil {
		t.Fatal(err)
	}

	if err := registry.unschedule(endpoint2, id2); err != nil {
		t.Fatal(err)
	}

	if want, have := 0, len(<-requestc); want != have {
		t.Fatalf("want %d, have %d", want, have)
	}
}

func TestRegistrySaveLoad(t *testing.T) {
	var (
		filename  = "registry-test-save-load.json"
		registry1 = newRealRegistry(filename)
		endpoint  = "http://314159.de"
		spec      = taskSpec{Endpoint: endpoint, ContainerID: "π"}
	)

	defer os.Remove(filename)

	defer registry1.quit()

	// Schedule a thing.

	if err := registry1.schedule(spec); err != nil {
		t.Fatal(err)
	}

	// Verify it persisted.

	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	var fromDisk map[string]taskSpec
	if err := json.Unmarshal(buf, &fromDisk); err != nil {
		t.Fatal(err)
	}

	check1c := make(chan map[string]taskSpec)
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

	check2c := make(chan map[string]taskSpec)
	registry2.subscribe(check2c)
	defer registry2.unsubscribe(check2c)

	if want, have := fromDisk, <-check2c; !reflect.DeepEqual(want, have) {
		t.Fatalf("want %v, have %v", want, have)
	}
}

func verifySpecs(t *testing.T, have map[string]taskSpec, s ...string) error {
	if len(s)%2 != 0 {
		return fmt.Errorf("bad invocation of verifySpecs")
	}

	var want = map[string]map[string]struct{}{} // endpoint: IDs

	for i := 0; i < len(s); i += 2 {
		endpoint, id := s[i], s[i+1]

		if _, ok := want[endpoint]; !ok {
			want[endpoint] = map[string]struct{}{}
		}

		want[endpoint][id] = struct{}{}
	}

	for endpoint, ids := range want {
		for id := range ids {
			spec, ok := have[id]
			if !ok {
				return fmt.Errorf("missing container %q", id)
			}

			if spec.Endpoint != endpoint {
				return fmt.Errorf("want %q on %s, but it's on %s", id, endpoint, spec.Endpoint)
			}

			t.Logf("%q on %s OK", id, endpoint)
		}
	}

	return nil
}
