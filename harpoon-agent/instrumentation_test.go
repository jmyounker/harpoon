package main

import (
	"expvar"
	"strconv"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestReceiveLogInstrumentation(t *testing.T) {
	registry := newRegistry()
	createReceiveLogsFixture(t, registry)
	c := newFakeContainer("123")
	registry.register(c)
	linec := make(chan string, 10) // Plenty of room before anything gets dropped
	c.Logs().notify(linec)

	clearCounters()
	sendLog("container[123] m1")
	waitForLogLine(t, linec, 100*time.Millisecond)
	ExpectCounterEqual(t, "log_received_lines", 1)
	ExpectCounterEqual(t, "log_unparsable_lines", 0)
	ExpectCounterEqual(t, "log_unroutable_lines", 0)

	clearCounters()
	sendLog("container[23] m2")
	expectNoLogLines(t, linec, 100*time.Millisecond)
	ExpectCounterEqual(t, "log_received_lines", 1)
	ExpectCounterEqual(t, "log_unparsable_lines", 0)
	ExpectCounterEqual(t, "log_unroutable_lines", 1)

	clearCounters()
	sendLog("ilj;irtr")
	expectNoLogLines(t, linec, 100*time.Millisecond)
	ExpectCounterEqual(t, "log_received_lines", 1)
	ExpectCounterEqual(t, "log_unparsable_lines", 1)
	ExpectCounterEqual(t, "log_unroutable_lines", 0)
}

func TestLogInstrumentationNotifyWithoutWatchers(t *testing.T) {
	registry := newRegistry()
	createReceiveLogsFixture(t, registry)

	registry.register(newFakeContainer("123"))

	// Create a second container which shouldn't receive any notifications
	// for the first channel.  This channel
	nonDestinationContainer := newFakeContainer("456")
	registry.register(nonDestinationContainer)
	nonDestinationLinec := make(chan string, 1)
	nonDestinationContainer.Logs().notify(nonDestinationLinec)

	clearCounters()
	sendLog("container[123] m1")
	expectNoLogLines(t, nonDestinationLinec, 100*time.Millisecond)
	ExpectCounterEqual(t, "log_received_lines", 1)
	ExpectCounterEqual(t, "log_unparsable_lines", 0)
	ExpectCounterEqual(t, "log_unroutable_lines", 0)
	ExpectCounterEqual(t, "log_deliverable_lines", 0)
	ExpectCounterEqual(t, "log_undelivered_lines", 0)
}

func TestLogInstrumentationNotifyWatchers(t *testing.T) {
	registry := newRegistry()
	createReceiveLogsFixture(t, registry)

	c := newFakeContainer("123")
	registry.register(c)
	linec1 := make(chan string, 1)
	linec2 := make(chan string, 1)
	c.Logs().notify(linec1)
	c.Logs().notify(linec2)

	clearCounters()
	sendLog("container[123] m1")
	waitForLogLine(t, linec1, 100*time.Millisecond)
	waitForLogLine(t, linec2, 100*time.Millisecond)
	ExpectCounterEqual(t, "log_received_lines", 1)
	ExpectCounterEqual(t, "log_unparsable_lines", 0)
	ExpectCounterEqual(t, "log_unroutable_lines", 0)
	ExpectCounterEqual(t, "log_deliverable_lines", 2)
	ExpectCounterEqual(t, "log_undelivered_lines", 0)
}

func TestLogInstrumentationNotifyWithBlockedWatcher(t *testing.T) {
	registry := newRegistry()
	createReceiveLogsFixture(t, registry)

	c := newFakeContainer("123")
	registry.register(c)
	linec1 := make(chan string, 1)
	linec2 := make(chan string) // Blocked channel
	c.Logs().notify(linec1)
	c.Logs().notify(linec2)

	clearCounters()
	sendLog("container[123] m1")
	waitForLogLine(t, linec1, 100*time.Millisecond)
	expectNoLogLines(t, linec2, 100*time.Millisecond)
	ExpectCounterEqual(t, "log_received_lines", 1)
	ExpectCounterEqual(t, "log_unparsable_lines", 0)
	ExpectCounterEqual(t, "log_unroutable_lines", 0)
	ExpectCounterEqual(t, "log_deliverable_lines", 1)
	ExpectCounterEqual(t, "log_undelivered_lines", 1)
}

func TestHeartbeatInstrumentation(t *testing.T) {
	clearCounters()
	c, hb := createHeartbeatFixture("DOWN", "UP", time.Now().Add(-time.Hour))
	c.Heartbeat(hb)
	ExpectCounterEqual(t, "container_status_kill", 1)
	ExpectCounterEqual(t, "container_status_down_successful", 0)
	ExpectCounterEqual(t, "container_status_force_down_successful", 0)

	clearCounters()
	c, hb = createHeartbeatFixture("DOWN", "UP", time.Now().Add(time.Hour))
	c.Heartbeat(hb)
	ExpectCounterEqual(t, "container_status_kill", 0)
	ExpectCounterEqual(t, "container_status_down_successful", 0)
	ExpectCounterEqual(t, "container_status_force_down_successful", 0)

	clearCounters()
	c, hb = createHeartbeatFixture("DOWN", "DOWN", time.Now().Add(time.Hour))
	c.Heartbeat(hb)
	ExpectCounterEqual(t, "container_status_kill", 0)
	ExpectCounterEqual(t, "container_status_down_successful", 1)
	ExpectCounterEqual(t, "container_status_force_down_successful", 0)

	clearCounters()
	c, hb = createHeartbeatFixture("FORCEDOWN", "DOWN", time.Now().Add(time.Hour))
	c.Heartbeat(hb)
	ExpectCounterEqual(t, "container_status_kill", 0)
	ExpectCounterEqual(t, "container_status_down_successful", 0)
	ExpectCounterEqual(t, "container_status_force_down_successful", 1)
}

func createHeartbeatFixture(desired string, have string, downDeadline time.Time) (*realContainer, agent.Heartbeat) {
	cc := agent.ContainerConfig{}
	c := newContainer("123", cc)
	c.desired = desired
	c.downDeadline = downDeadline
	return c, agent.Heartbeat{Status: have}
}

func createReceiveLogsFixture(t *testing.T, r *registry) {
	setLogAddrRandomly(t)
	go receiveLogs(r)
}

func ExpectCounterEqual(t *testing.T, name string, value int) {
	expvarValue, err := strconv.Atoi(expvar.Get(name).String())
	if err != nil {
		t.Fatalf("unable to convert counter %s to an int: %s", name, err)
	}
	if expvarValue != value {
		t.Errorf("Expected expvar %q to have value %d instead of %d", name, value, expvarValue)
	}
	prometheusCounter := readCounter(expvarToPrometheusCounter[name])
	if prometheusCounter != float64(value) {
		t.Errorf("Expected expvar %q to have value %f instead of %f", name, float64(value), prometheusCounter)
	}
}

func readCounter(m prometheus.Counter) float64 {
	pb := &dto.Metric{}
	m.Write(pb)
	return pb.GetCounter().GetValue()
}

var (
	expvarToPrometheusCounter = map[string]prometheus.Counter{
		"log_received_lines":                     prometheusLogReceivedLines,
		"log_unparsable_lines":                   prometheusLogUnparsableLines,
		"log_unroutable_lines":                   prometheusLogUnroutableLines,
		"log_deliverable_lines":                  prometheusLogDeliverableLines,
		"log_undelivered_lines":                  prometheusLogUndeliveredLines,
		"container_status_kill":                  prometheusContainerStatusKilled,
		"container_status_down_successful":       prometheusContainerStatusDownSuccessful,
		"container_status_force_down_successful": prometheusContainerStatusForceDownSuccessful,
	}
)

func clearCounters() {
	for name, prometheusCounter := range expvarToPrometheusCounter {
		expvar.Get(name).(*expvar.Int).Set(0)
		prometheusCounter.Set(0)
	}
}
