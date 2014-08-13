package main

import (
	"expvar"

	"github.com/prometheus/client_golang/prometheus"
)

// A log line moves through the following states:
//   - A log line is read from the logging port. The line has been RECEIVED.
//   - The RECEIVED line is matched with a container.
//      - The line has been ROUTED to a container or it is UNROUTED.
//   - A ROUTED line is sent to all channels listening to that container's logs.
//       - Each copy potentially sent to a listener's channel is DELIVERABLE.
//       - Each DELIVERABLE which encountered a blocked channel is UNDELIVERED.
//
// So...
//   - Every inbound message generates a received count.
//   - unparsable count + unrouted count + routed count = received count
//   - deliverable = sum(containers[x].listeners.count * containers[x].routed_to)[1..#containers]
//   - delivered count + undelivered count = deliverable
//
var (
	expvarLogReceivedLines                   = expvar.NewInt("log_received_lines")
	expvarLogUnparsableLines                 = expvar.NewInt("log_unparsable_lines")
	expvarLogUnroutableLines                 = expvar.NewInt("log_unroutable_lines")
	expvarLogDeliverableLines                = expvar.NewInt("log_deliverable_lines")
	expvarLogUndeliveredLines                = expvar.NewInt("log_undelivered_lines")
	expvarContainerCreate                    = expvar.NewInt("container_create")
	expvarContainerCreateFailure             = expvar.NewInt("container_create_failure")
	expvarContainerDestroy                   = expvar.NewInt("container_destroy")
	expvarContainerStart                     = expvar.NewInt("container_start")
	expvarContainerStartFailure              = expvar.NewInt("container_start_failure")
	expvarContainerStop                      = expvar.NewInt("container_stop")
	expvarContainerStatusKilled              = expvar.NewInt("container_status_kill")
	expvarContainerStatusDownSuccessful      = expvar.NewInt("container_status_down_successful")
	expvarContainerStatusForceDownSuccessful = expvar.NewInt("container_status_force_down_successful")
)

// Derivable metrics:
//   RoutedLines = ReceivedLines - UnparsableLines - UnroutedLines
//   DeliveredLines = DeliverableLines - UndeliveredLines

var (
	prometheusLogReceivedLines = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "agent",
		Name:      "log_received_lines",
		Help:      "Number of log lines received from external sources.",
	})
	prometheusLogUnparsableLines = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "agent",
		Name:      "log_unparsable_lines",
		Help:      "Number of log lines which could not be routed to a container.",
	})
	prometheusLogUnroutableLines = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "agent",
		Name:      "log_unroutable_lines",
		Help:      "Number of log lines which could not be routed to a container.",
	})
	prometheusLogDeliverableLines = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "agent",
		Name:      "log_deliverable_lines",
		Help:      "Number of accepted log lines not written because of a blocked listener.",
	})
	prometheusLogUndeliveredLines = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "agent",
		Name:      "log_undelivered_lines",
		Help:      "Number of accepted log lines written to listeners.",
	})
	prometheusContainerCreate = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "agent",
		Name:      "container_create",
		Help:      "Number of times the agent has created a container.",
	})
	prometheusContainerCreateFailure = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "agent",
		Name:      "container_create_failure",
		Help:      "Number of times that the container create operation failed.",
	})
	prometheusContainerDestroy = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "agent",
		Name:      "container_destroy",
		Help:      "Number of times agent has attempted to destroy a container",
	})
	prometheusContainerStart = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "agent",
		Name:      "container_start",
		Help:      "Number of times an agent start command was sent to a container",
	})
	prometheusContainerStartFailure = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "agent",
		Name:      "container_start_failure",
		Help:      "Number of times a container start operation has failed.",
	})
	prometheusContainerStop = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "agent",
		Name:      "container_stop",
		Help:      "Number of times the agent attempted stop a container.",
	})
	prometheusContainerStatusKilled = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "agent",
		Name:      "container_status_killed",
		Help:      "Number of times that a container had to be killed forcibly.",
	})
	prometheusContainerStatusDownSuccessful = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "agent",
		Name:      "container_status_down_successful",
		Help:      "Number of times that a container was successfully shut down.",
	})
	prometheusContainerStatusForceDownSuccessful = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "agent",
		Name:      "container_status_force_down_successful",
		Help:      "Number of times that a container was successfully forced down.",
	})
)

func incLogReceivedLines(n int) {
	expvarLogReceivedLines.Add(int64(n))
	prometheusLogReceivedLines.Add(float64(n))
}

func incLogUnparsableLines(n int) {
	expvarLogUnparsableLines.Add(int64(n))
	prometheusLogUnparsableLines.Add(float64(n))
}

func incLogUnroutableLines(n int) {
	expvarLogUnroutableLines.Add(int64(n))
	prometheusLogUnroutableLines.Add(float64(n))
}

func incLogDeliverableLines(n int) {
	expvarLogDeliverableLines.Add(int64(n))
	prometheusLogDeliverableLines.Add(float64(n))
}

func incLogUndeliveredLines(n int) {
	expvarLogUndeliveredLines.Add(int64(n))
	prometheusLogUndeliveredLines.Add(float64(n))
}

func incContainerStart(n int) {
	expvarContainerStart.Add(int64(n))
	prometheusContainerStart.Add(float64(n))
}

func incContainerStartFailure(n int) {
	expvarContainerStartFailure.Add(int64(n))
	prometheusContainerStartFailure.Add(float64(n))
}

func incContainerDestroy(n int) {
	expvarContainerDestroy.Add(int64(n))
	prometheusContainerDestroy.Add(float64(n))
}

func incContainerCreate(n int) {
	expvarContainerCreate.Add(int64(n))
	prometheusContainerCreate.Add(float64(n))
}

func incContainerCreateFailure(n int) {
	expvarContainerCreateFailure.Add(int64(n))
	prometheusContainerCreateFailure.Add(float64(n))
}

func incContainerStop(n int) {
	expvarContainerStop.Add(int64(n))
	prometheusContainerStop.Add(float64(n))
}

func incContainerStatusKilled(n int) {
	expvarContainerStatusKilled.Add(int64(n))
	prometheusContainerStatusKilled.Add(float64(n))
}

func incContainerStatusDownSuccessful(n int) {
	expvarContainerStatusDownSuccessful.Add(int64(n))
	prometheusContainerStatusDownSuccessful.Add(float64(n))
}

func incContainerStatusForceDownSuccessful(n int) {
	expvarContainerStatusForceDownSuccessful.Add(int64(n))
	prometheusContainerStatusForceDownSuccessful.Add(float64(n))
}
