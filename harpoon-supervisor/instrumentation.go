package main

import (
	"expvar"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

var (
	expvarContainerRestart     = expvar.NewInt("container_restarts_total")
	expvarContainerOOM         = expvar.NewInt("container_ooms_total")
	expvarContainerCPUTime     = expvar.NewInt("container_cpu_time_ns")
	expvarContainerMemoryUsage = expvar.NewInt("container_memory_usage_bytes")
	expvarContainerMemoryLimit = expvar.NewInt("container_memory_limit_bytes")

	prometheusContainerRestart     prometheus.Counter
	prometheusContainerOOM         prometheus.Counter
	prometheusContainerCPUTime     prometheus.Gauge
	prometheusContainerMemoryUsage prometheus.Gauge
	prometheusContainerMemoryLimit prometheus.Gauge
)

func setupMetrics(labels prometheus.Labels) {
	prometheusContainerRestart = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   "harpoon",
		Subsystem:   "supervisor",
		Name:        "container_restarts_total",
		Help:        "Counter of restarts for a container.",
		ConstLabels: labels,
	})
	prometheus.MustRegister(prometheusContainerRestart)

	prometheusContainerOOM = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   "harpoon",
		Subsystem:   "supervisor",
		Name:        "container_ooms_total",
		Help:        "Counter of OOM events for container.",
		ConstLabels: labels,
	})
	prometheus.MustRegister(prometheusContainerOOM)

	prometheusContainerCPUTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   "harpoon",
		Subsystem:   "supervisor",
		Name:        "container_cpu_time_ns",
		Help:        "CPU time of a container (in nanoseconds).",
		ConstLabels: labels,
	})
	prometheus.MustRegister(prometheusContainerCPUTime)

	prometheusContainerMemoryUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   "harpoon",
		Subsystem:   "supervisor",
		Name:        "container_memory_usage_bytes",
		Help:        "Physical memory consumed by the container.",
		ConstLabels: labels,
	})
	prometheus.MustRegister(prometheusContainerMemoryUsage)

	prometheusContainerMemoryLimit = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   "harpoon",
		Subsystem:   "supervisor",
		Name:        "container_memory_limit_bytes",
		Help:        "Memory reserved by container in bytes.",
		ConstLabels: labels,
	})
	prometheus.MustRegister(prometheusContainerMemoryLimit)
}

func incContainerRestart(n int) {
	expvarContainerRestart.Add(int64(n))
	prometheusContainerRestart.Add(float64(n))
}

func incContainerOOM(n int) {
	expvarContainerOOM.Add(int64(n))
	prometheusContainerOOM.Add(float64(n))
}

func updateMetrics(m agent.ContainerMetrics) {
	expvarContainerCPUTime.Set(int64(m.CPUTime))
	prometheusContainerCPUTime.Set(float64(m.CPUTime))

	expvarContainerMemoryUsage.Set(int64(m.MemoryUsage))
	prometheusContainerMemoryUsage.Set(float64(m.MemoryUsage))

	expvarContainerMemoryLimit.Set(int64(m.MemoryLimit))
	prometheusContainerMemoryLimit.Set(float64(m.MemoryLimit))
}
