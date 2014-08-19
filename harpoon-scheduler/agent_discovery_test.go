package main

import (
	"reflect"
	"testing"
)

func TestStaticAgentDiscovery(t *testing.T) {
	const (
		endpoint1 = "http://computers.berlin:31337"
		endpoint2 = "http://kraftwerk.info:8080"
	)

	var (
		discovery = staticAgentDiscovery{endpoint1, endpoint2}
		updatec   = make(chan []string)
		requestc  = make(chan []string)
	)

	discovery.subscribe(updatec)
	defer close(updatec)
	defer discovery.unsubscribe(updatec)

	go func() {
		endpoints, ok := <-updatec
		if !ok {
			return
		}

		for {
			select {
			case requestc <- endpoints:
			case endpoints, ok = <-updatec:
				if !ok {
					return
				}
			}
		}
	}()

	if want, have := []string{endpoint1, endpoint2}, <-requestc; !reflect.DeepEqual(want, have) {
		t.Errorf("want %v, have %v", want, have)
	}
}

func TestManualAgentDiscovery(t *testing.T) {
	const (
		endpoint1 = "http://computers.berlin:31337"
		endpoint2 = "http://kraftwerk.info:8080"
		endpoint3 = "http://hats.for.cats.biz"
	)

	var (
		discovery = newManualAgentDiscovery([]string{endpoint1})
		updatec   = make(chan []string)
		requestc  = make(chan []string)
	)

	discovery.subscribe(updatec)
	defer close(updatec)
	defer discovery.unsubscribe(updatec)

	go func() {
		endpoints, ok := <-updatec
		if !ok {
			return
		}

		for {
			select {
			case requestc <- endpoints:
			case endpoints, ok = <-updatec:
				if !ok {
					return
				}
			}
		}
	}()

	if want, have := []string{endpoint1}, <-requestc; !reflect.DeepEqual(want, have) {
		t.Errorf("want %v, have %v", want, have)
	}

	discovery.add(endpoint2)

	if want, have := []string{endpoint1, endpoint2}, <-requestc; !reflect.DeepEqual(want, have) {
		t.Errorf("want %v, have %v", want, have)
	}

	discovery.add(endpoint3)

	if want, have := []string{endpoint1, endpoint2, endpoint3}, <-requestc; !reflect.DeepEqual(want, have) {
		t.Errorf("want %v, have %v", want, have)
	}

	discovery.del(endpoint1)

	if want, have := []string{endpoint2, endpoint3}, <-requestc; !reflect.DeepEqual(want, have) {
		t.Errorf("want %v, have %v", want, have)
	}

	discovery.del(endpoint2)

	if want, have := []string{endpoint3}, <-requestc; !reflect.DeepEqual(want, have) {
		t.Errorf("want %v, have %v", want, have)
	}

	discovery.del(endpoint3)

	if want, have := []string{}, <-requestc; !reflect.DeepEqual(want, have) {
		t.Errorf("want %v, have %v", want, have)
	}
}
