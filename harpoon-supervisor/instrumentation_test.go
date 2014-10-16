package main

import (
	"expvar"
	"fmt"
	"strconv"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/soundcloud/harpoon/harpoon-agent/lib"

	"github.com/prometheus/client_golang/prometheus"
)

func TestRestartInstrumentation(t *testing.T) {
	var (
		container    = newFakeContainer()
		supervisor   = newSupervisor(container)
		restartTimer = make(chan time.Time)
	)
	clearCounters()

	go func() {
		supervisor.Run(nil, func() <-chan time.Time {
			return restartTimer
		})
	}()

	select {
	case container.startc <- nil:
	case <-time.After(time.Millisecond):
		t.Fatal("supervisor did not attempt to start container")
	}

	for i := 1; i <= 2; i++ {
		select {
		case container.waitc <- agent.ContainerExitStatus{OOMed: true}:
		case <-time.After(time.Millisecond):
			t.Fatalf("%d: supervisor did not consume container exit status", i)
		}

		select {
		case restartTimer <- time.Now():
		case <-time.After(time.Millisecond):
			t.Fatalf("%d: supervisor did not consume restart message", i)
		}

		select {
		case container.startc <- nil:
		case <-time.After(time.Millisecond):
			t.Fatalf("%d: supervisor did not attempt to start container", i)
		}

		if err := expectCounterEqual("container_restarts_total", i); err != nil {
			t.Fatalf("%d:%v", i, err)
		}
	}
}

func TestOOMInstrumentation(t *testing.T) {
	var (
		container  = newFakeContainer()
		supervisor = newSupervisor(container)
	)
	clearCounters()

	go func() {
		supervisor.Run(nil, func() <-chan time.Time {
			return nil
		})
	}()

	if err := expectCounterEqual("container_ooms_total", 0); err != nil {
		t.Fatal(err)
	}

	select {
	case container.startc <- nil:
	case <-time.After(time.Millisecond):
		t.Fatal("supervisor did not attempt to start container")
	}

	select {
	case container.waitc <- agent.ContainerExitStatus{OOMed: true}:
	case <-time.After(time.Millisecond):
		t.Fatal("supervisor did not consume exit status")
	}

	time.Sleep(time.Millisecond)

	if err := expectCounterEqual("container_ooms_total", 1); err != nil {
		t.Fatal(err)
	}
}

func TestMetricsInstrumentation(t *testing.T) {
	var (
		container   = newFakeContainer()
		supervisor  = newSupervisor(container)
		metricsTick = make(chan time.Time)
	)
	clearCounters()

	go func() {
		supervisor.Run(metricsTick, func() <-chan time.Time {
			return nil
		})
	}()

	select {
	case container.startc <- nil:
	case <-time.After(time.Millisecond):
		t.Fatal("supervisor did not attempt to start container")
	}

	for i := 1; i <= 2; i++ {
		container.metrics = uint64(i)
		select {
		case metricsTick <- time.Now():
		case <-time.After(time.Millisecond):
			t.Fatalf("%d: supervisor did not consume metrics tick", i)
		}

		time.Sleep(time.Millisecond)

		for metric := range expvarToPrometheusGauge {
			if err := expectGaugeEqual(metric, i); err != nil {
				t.Fatalf("%d: incorrect metric %q error: %v", i, metric, err)
			}
		}
	}
}

var (
	expvarToPrometheusCounter map[string]prometheus.Counter
	expvarToPrometheusGauge   map[string]prometheus.Gauge
)

func init() {
	setupMetrics(prometheus.Labels(telemetryLabels{"key": "value"}))

	expvarToPrometheusCounter = map[string]prometheus.Counter{
		"container_restarts_total": prometheusContainerRestart,
		"container_ooms_total":     prometheusContainerOOM,
	}

	expvarToPrometheusGauge = map[string]prometheus.Gauge{
		"container_cpu_time_ns":        prometheusContainerCPUTime,
		"container_memory_usage_bytes": prometheusContainerMemoryUsage,
		"container_memory_limit_bytes": prometheusContainerMemoryLimit,
	}
}

func expectCounterEqual(name string, value int) error {
	expvarValue, err := strconv.Atoi(expvar.Get(name).String())
	if err != nil {
		return fmt.Errorf("unable to convert counter %s to an int: %s", name, err)
	}
	if expvarValue != value {
		return fmt.Errorf("expected expvar %q to have value %d instead of %d", name, value, expvarValue)
	}

	pb := &dto.Metric{}
	counter, ok := expvarToPrometheusCounter[name]
	if !ok {
		return fmt.Errorf("invalid name %q", name)
	}
	counter.Write(pb)
	prometheusCounter := pb.GetCounter().GetValue()

	if prometheusCounter != float64(value) {
		return fmt.Errorf("expected expvar %q to have value %f instead of %f", name, float64(value), prometheusCounter)
	}

	return nil
}

func expectGaugeEqual(name string, value int) error {
	expvarValue, err := strconv.Atoi(expvar.Get(name).String())
	if err != nil {
		return fmt.Errorf("unable to convert counter %s to an int: %s", name, err)
	}
	if expvarValue != value {
		return fmt.Errorf("expected expvar %q to have value %d instead of %d", name, value, expvarValue)
	}

	pb := &dto.Metric{}
	gauge, ok := expvarToPrometheusGauge[name]
	if !ok {
		return fmt.Errorf("invalid name %q", name)
	}
	gauge.Write(pb)
	prometheusGauge := pb.GetGauge().GetValue()

	if prometheusGauge != float64(value) {
		return fmt.Errorf("expected expvar %q to have value %f instead of %f", name, float64(value), prometheusGauge)
	}

	return nil
}

func clearCounters() {
	for name, prometheusCounter := range expvarToPrometheusCounter {
		expvar.Get(name).(*expvar.Int).Set(0)
		prometheusCounter.Set(0)
	}

	for name, prometheusGauge := range expvarToPrometheusGauge {
		expvar.Get(name).(*expvar.Int).Set(0)
		prometheusGauge.Set(0)
	}
}
