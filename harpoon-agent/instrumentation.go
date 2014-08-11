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
	expvarLogReceivedLines = expvar.NewInt("log_received_lines")
	expvarLogUnparsableLines = expvar.NewInt("log_unparsable_lines")
	expvarLogUnroutableLines   = expvar.NewInt("log_unroutable_lines")
	expvarLogDeliverableLines = expvar.NewInt("log_deliverable_lines")
	expvarLogUndeliveredLines  = expvar.NewInt("log_undelivered_lines")
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
