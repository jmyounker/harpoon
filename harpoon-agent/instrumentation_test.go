package main

import (
	"expvar"
	"strconv"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/client_golang/prometheus"
)

func TestReceiveLogInstrumentation(t *testing.T) {
	registry := newRegistry()
	createReceiveLogsFixture(t, registry)
	c := newFakeContainer("123")
	registry.register(c)
	linec := make(chan string, 10) // Plenty of room before anything gets dropped
	c.Logs().notify(linec)

	clearLogCounters()
	sendLog("container[123] m1")
	waitForLogLine(t, linec, 100*time.Millisecond)
	ExpectCounterEqual(t, "log_received_lines", 1)
	ExpectCounterEqual(t, "log_unparsable_lines", 0)
	ExpectCounterEqual(t, "log_unroutable_lines", 0)

	clearLogCounters()
	sendLog("container[23] m2")
	expectNoLogLines(t, linec, 100*time.Millisecond)
	ExpectCounterEqual(t, "log_received_lines", 1)
	ExpectCounterEqual(t, "log_unparsable_lines", 0)
	ExpectCounterEqual(t, "log_unroutable_lines", 1)

	clearLogCounters()
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

	clearLogCounters()
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

	clearLogCounters()
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

	clearLogCounters()
	sendLog("container[123] m1")
	waitForLogLine(t, linec1, 100*time.Millisecond)
	expectNoLogLines(t, linec2, 100*time.Millisecond)
	ExpectCounterEqual(t, "log_received_lines", 1)
	ExpectCounterEqual(t, "log_unparsable_lines", 0)
	ExpectCounterEqual(t, "log_unroutable_lines", 0)
	ExpectCounterEqual(t, "log_deliverable_lines", 1)
	ExpectCounterEqual(t, "log_undelivered_lines", 1)
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
	prometheusCounter := readCounter(expvarToPrometheusLogCounter(name))
	if prometheusCounter != float64(value) {
		t.Errorf("Expected expvar %q to have value %f instead of %f", name, float64(value), prometheusCounter)
	}
}

func readCounter(m prometheus.Counter) float64 {
	pb := &dto.Metric{}
	m.Write(pb)
	return pb.GetCounter().GetValue()
}

func expvarToPrometheusLogCounter(name string) prometheus.Counter {
	switch name {
	case "log_received_lines":
		return prometheusLogReceivedLines
	case "log_unparsable_lines":
		return prometheusLogUnparsableLines
	case "log_unroutable_lines":
		return prometheusLogUnroutableLines
	case "log_deliverable_lines":
		return prometheusLogDeliverableLines
	case "log_undelivered_lines":
		return prometheusLogUndeliveredLines
	default:
		panic("Missing counter name")
	}
}

func clearLogCounters() {
	expvar.Get("log_received_lines").(*expvar.Int).Set(0)
	expvar.Get("log_unparsable_lines").(*expvar.Int).Set(0)
	expvar.Get("log_unroutable_lines").(*expvar.Int).Set(0)
	expvar.Get("log_deliverable_lines").(*expvar.Int).Set(0)
	expvar.Get("log_undelivered_lines").(*expvar.Int).Set(0)

	prometheusLogReceivedLines.Set(0)
	prometheusLogUnparsableLines.Set(0)
	prometheusLogUnroutableLines.Set(0)
	prometheusLogDeliverableLines.Set(0)
	prometheusLogUndeliveredLines.Set(0)
}
