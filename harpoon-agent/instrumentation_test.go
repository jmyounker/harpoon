package main

import (
	"expvar"
	"fmt"
	"strconv"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestReceiveLogInstrumentation(t *testing.T) {
	registry := newRegistry(nopServiceDiscovery{})
	createReceiveLogsFixture(t, registry)
	c := newFakeContainer("123", "", volumes{}, agent.ContainerConfig{}, false, nil)
	registry.register(c)
	linec := make(chan string, 10) // Plenty of room before anything gets dropped
	c.Logs().notify(linec)

	clearCounters()
	sendLog("container[123] m1")
	waitForLogLine(t, linec, 100*time.Millisecond)
	expectCounterEqual(t, "log_received_lines_total", 1)
	expectCounterEqual(t, "log_unparsable_lines_total", 0)
	expectCounterEqual(t, "log_unroutable_lines_total", 0)

	clearCounters()
	sendLog("container[23] m2")
	expectNoLogLines(t, linec, 100*time.Millisecond)
	expectCounterEqual(t, "log_received_lines_total", 1)
	expectCounterEqual(t, "log_unparsable_lines_total", 0)
	expectCounterEqual(t, "log_unroutable_lines_total", 1)

	clearCounters()
	sendLog("ilj;irtr")
	expectNoLogLines(t, linec, 100*time.Millisecond)
	expectCounterEqual(t, "log_received_lines_total", 1)
	expectCounterEqual(t, "log_unparsable_lines_total", 1)
	expectCounterEqual(t, "log_unroutable_lines_total", 0)
}

func TestLogInstrumentationNotifyWithoutWatchers(t *testing.T) {
	registry := newRegistry(nopServiceDiscovery{})
	createReceiveLogsFixture(t, registry)

	registry.register(newFakeContainer("123", "", volumes{}, agent.ContainerConfig{}, false, nil))

	// Create a second container which shouldn't receive any notifications
	// for the first channel.  This channel
	nonDestinationContainer := newFakeContainer("456", "", volumes{}, agent.ContainerConfig{}, false, nil)
	registry.register(nonDestinationContainer)
	nonDestinationLinec := make(chan string, 1)
	nonDestinationContainer.Logs().notify(nonDestinationLinec)

	clearCounters()
	sendLog("container[123] m1")
	expectNoLogLines(t, nonDestinationLinec, 100*time.Millisecond)
	expectCounterEqual(t, "log_received_lines_total", 1)
	expectCounterEqual(t, "log_unparsable_lines_total", 0)
	expectCounterEqual(t, "log_unroutable_lines_total", 0)
	expectCounterEqual(t, "log_deliverable_lines_total", 0)
	expectCounterEqual(t, "log_undelivered_lines_total", 0)
}

func TestLogInstrumentationNotifyWatchers(t *testing.T) {
	registry := newRegistry(nopServiceDiscovery{})
	createReceiveLogsFixture(t, registry)

	c := newFakeContainer("123", "", volumes{}, agent.ContainerConfig{}, false, nil)
	registry.register(c)
	linec1 := make(chan string, 1)
	linec2 := make(chan string, 1)
	c.Logs().notify(linec1)
	c.Logs().notify(linec2)

	clearCounters()
	sendLog("container[123] m1")
	waitForLogLine(t, linec1, 100*time.Millisecond)
	waitForLogLine(t, linec2, 100*time.Millisecond)
	expectCounterEqual(t, "log_received_lines_total", 1)
	expectCounterEqual(t, "log_unparsable_lines_total", 0)
	expectCounterEqual(t, "log_unroutable_lines_total", 0)
	expectCounterEqual(t, "log_deliverable_lines_total", 2)
	expectCounterEqual(t, "log_undelivered_lines_total", 0)
}

func TestLogInstrumentationNotifyWithBlockedWatcher(t *testing.T) {
	registry := newRegistry(nopServiceDiscovery{})
	createReceiveLogsFixture(t, registry)

	c := newFakeContainer("123", "", volumes{}, agent.ContainerConfig{}, false, nil)
	registry.register(c)
	linec1 := make(chan string, 1)
	linec2 := make(chan string) // Blocked channel
	c.Logs().notify(linec1)
	c.Logs().notify(linec2)

	clearCounters()
	sendLog("container[123] m1")
	waitForLogLine(t, linec1, 100*time.Millisecond)
	expectNoLogLines(t, linec2, 100*time.Millisecond)
	expectCounterEqual(t, "log_received_lines_total", 1)
	expectCounterEqual(t, "log_unparsable_lines_total", 0)
	expectCounterEqual(t, "log_unroutable_lines_total", 0)
	expectCounterEqual(t, "log_deliverable_lines_total", 1)
	expectCounterEqual(t, "log_undelivered_lines_total", 1)
}

func expectCounterEqual(t *testing.T, name string, value int) {
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
		"log_received_lines_total":                     prometheusLogReceivedLines,
		"log_unparsable_lines_total":                   prometheusLogUnparsableLines,
		"log_unroutable_lines_total":                   prometheusLogUnroutableLines,
		"log_deliverable_lines_total":                  prometheusLogDeliverableLines,
		"log_undelivered_lines_total":                  prometheusLogUndeliveredLines,
		"container_status_kill_total":                  prometheusContainerStatusKilled,
		"container_status_down_successful_total":       prometheusContainerStatusDownSuccessful,
		"container_status_force_down_successful_total": prometheusContainerStatusForceDownSuccessful,
	}
)

func clearCounters() {
	for name, prometheusCounter := range expvarToPrometheusCounter {
		v := expvar.Get(name)
		if v == nil {
			panic(fmt.Sprintf("invalid name %q", name))
		}
		v.(*expvar.Int).Set(0)
		prometheusCounter.Set(0)
	}
}
