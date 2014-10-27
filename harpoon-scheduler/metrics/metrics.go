// Package metrics provides methods to instrument the scheduler. Metrics are
// provided simultaneously in expvar and Prometheus "formats". expvar is
// automatically mounted in http.DefaultServeMux /debug/vars. Clients need to
// mount the Prometheus handler manually, via prometheus.Handler().
package metrics

import (
	"expvar"

	// Import to expose profiling endpoints.
	_ "net/http/pprof"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	expvarJobScheduleRequests         = expvar.NewInt("job_schedule_requests")
	expvarJobUnscheduleRequests       = expvar.NewInt("job_unschedule_requests")
	expvarTransformsExecuted          = expvar.NewInt("transforms_executed")
	expvarTransformsSkipped           = expvar.NewInt("transforms_skipped")
	expvarTransactionsCreated         = expvar.NewInt("transactions_created")
	expvarTransactionsResolved        = expvar.NewInt("transactions_resolved")
	expvarTransactionsFailed          = expvar.NewInt("transactions_failed")
	expvarContainersRequested         = expvar.NewInt("containers_requested")
	expvarContainersPlaced            = expvar.NewInt("containers_placed")
	expvarContainersFailed            = expvar.NewInt("containers_failed")
	expvarAgentsLost                  = expvar.NewInt("agents_lost")
	expvarAgentConnectionsEstablished = expvar.NewInt("agent_connections_established")
	expvarAgentConnectionsInterrupted = expvar.NewInt("agent_connections_interrupted")
	expvarContainerEventsReceived     = expvar.NewInt("container_events_received")
)

var (
	prometheusJobScheduleRequests = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "job_schedule_requests",
		Help:      "Number of job schedule requests received by the scheduler.",
	})
	prometheusJobUnscheduleRequests = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "job_unschedule_requests",
		Help:      "Number of job unschedule requests received by the scheduler.",
	})
	prometheusTransformsExecuted = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "transforms_executed",
		Help:      "Number of transform operations successfully executed by the scheduler transformer.",
	})
	prometheusTransformsSkipped = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "transforms_skipped",
		Help:      "Number of transform operations skipped due to semaphore contention.",
	})
	prometheusTransactionsCreated = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "transactions_created",
		Help:      "Number of schedule or unschedule transactions created.",
	})
	prometheusTransactionsResolved = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "transactions_resolved",
		Help:      "Number of schedule or unschedule transactions resolved successfully.",
	})
	prometheusTransactionsFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "transactions_failed",
		Help:      "Number of schedule or unschedule transactions that failed to resolve.",
	})
	prometheusContainersRequested = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "containers_requested",
		Help:      "Number of containers requested to be placed by a scheduling algorithm.",
	})
	prometheusContainersPlaced = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "containers_placed",
		Help:      "Number of containers successfully placed by a scheduling algorithm.",
	})
	prometheusContainersFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "containers_failed",
		Help:      "Number of containers failed to be placed by a scheduling algorithm.",
	})
	prometheusAgentsLost = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "agents_lost",
		Help:      "Number of agents lost completely from agent discovery.",
	})
	prometheusAgentConnectionsEstablished = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "agent_connections_established",
		Help:      "Number of event stream connections to remote agents that have been established.",
	})
	prometheusAgentConnectionsInterrupted = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "agent_connections_interrupted",
		Help:      "Number of event stream connections to remote agents that have been interrupted.",
	})
	prometheusContainerEventsReceived = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "container_events_received",
		Help:      "Number of complete events received from remote agents.",
	})
)

// IncJobScheduleRequests increments the number of requests to schedule new
// jobs.
func IncJobScheduleRequests(n int) {
	expvarJobScheduleRequests.Add(int64(n))
	prometheusJobScheduleRequests.Add(float64(n))
}

// IncJobUnscheduleRequests increments the number of requests to unschedule a
// job.
func IncJobUnscheduleRequests(n int) {
	expvarJobUnscheduleRequests.Add(int64(n))
	prometheusJobUnscheduleRequests.Add(float64(n))
}

// IncTransformsExecuted increments the number of transforms executed.
func IncTransformsExecuted(n int) {
	expvarTransformsExecuted.Add(int64(n))
	prometheusTransformsExecuted.Add(float64(n))
}

// IncTransformsSkipped increments the number of transforms skipped.
func IncTransformsSkipped(n int) {
	expvarTransformsSkipped.Add(int64(n))
	prometheusTransformsSkipped.Add(float64(n))
}

// IncTransactionsCreated increments the number of transactions created.
func IncTransactionsCreated(n int) {
	expvarTransactionsCreated.Add(int64(n))
	prometheusTransactionsCreated.Add(float64(n))
}

// IncTransactionsResolved increments the number of transactions successfully
// resolved.
func IncTransactionsResolved(n int) {
	expvarTransactionsResolved.Add(int64(n))
	prometheusTransactionsResolved.Add(float64(n))
}

// IncTransactionsFailed increments the number of transactions
// unsuccessfully resolved.
func IncTransactionsFailed(n int) {
	expvarTransactionsFailed.Add(int64(n))
	prometheusTransactionsFailed.Add(float64(n))
}

// IncContainersRequested increments the number of containers that were
// requested for placement by a scheduling algorithm.
func IncContainersRequested(n int) {
	expvarContainersRequested.Add(int64(n))
	prometheusContainersRequested.Add(float64(n))
}

// IncContainersPlaced increments the number of containers successfully
// placed by a scheduling algorithm.
func IncContainersPlaced(n int) {
	expvarContainersPlaced.Add(int64(n))
	prometheusContainersPlaced.Add(float64(n))
}

// IncContainersFailed increments the number of containers that weren't able
// to be placed by a scheduling algorithm.
func IncContainersFailed(n int) {
	expvarContainersFailed.Add(int64(n))
	prometheusContainersFailed.Add(float64(n))
}

// IncAgentsLost increments the number of times the scheduler has lost
// communication with an agent for long enough to consider its containers
// abandoned.
func IncAgentsLost(n int) {
	expvarAgentsLost.Add(int64(n))
	prometheusAgentsLost.Add(float64(n))
}

// IncAgentConnectionsEstablished increments the number of connections
// successfully established to remote agents. That can happen when a scheduler
// initially starts, or after an existing connection is interrupted.
func IncAgentConnectionsEstablished(n int) {
	expvarAgentConnectionsEstablished.Add(int64(n))
	prometheusAgentConnectionsEstablished.Add(float64(n))
}

// IncAgentConnectionsInterrupted increments the number of times a connection
// from the scheduler to a remote agent is interrupted.
func IncAgentConnectionsInterrupted(n int) {
	expvarAgentConnectionsInterrupted.Add(int64(n))
	prometheusAgentConnectionsInterrupted.Add(float64(n))
}

// IncContainerEventsReceived increments the number of event-stream events
// received by the scheduler from an agent.
func IncContainerEventsReceived(n int) {
	expvarContainerEventsReceived.Add(int64(n))
	prometheusContainerEventsReceived.Add(float64(n))
}
