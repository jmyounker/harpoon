package main

import (
	"expvar"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	expvarJobScheduleRequests         = expvar.NewInt("job_schedule_requests")
	expvarJobMigrateRequests          = expvar.NewInt("job_migrate_requests")
	expvarJobUnscheduleRequests       = expvar.NewInt("job_unschedule_requests")
	expvarTaskScheduleRequests        = expvar.NewInt("task_schedule_requests")
	expvarTaskUnscheduleRequests      = expvar.NewInt("task_unschedule_requests")
	expvarContainersPlaced            = expvar.NewInt("containers_placed")
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
	prometheusJobMigrateRequests = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "job_migrate_requests",
		Help:      "Number of job migrate requests received by the scheduler.",
	})
	prometheusJobUnscheduleRequests = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "job_unschedule_requests",
		Help:      "Number of job unschedule requests received by the scheduler.",
	})
	prometheusTaskScheduleRequests = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "task_schedule_requests",
		Help:      "Number of task schedule requests received by the transformer.",
	})
	prometheusTaskUnscheduleRequests = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "task_unschedule_requests",
		Help:      "Number of task unschedule requests received by the transformer.",
	})
	prometheusContainersPlaced = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "harpoon",
		Subsystem: "scheduler",
		Name:      "containers_placed",
		Help:      "Number of containers successfully placed by a scheduling algorithm.",
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

func incJobScheduleRequests(n int) {
	expvarJobScheduleRequests.Add(int64(n))
	prometheusJobScheduleRequests.Add(float64(n))
}

func incJobMigrateRequests(n int) {
	expvarJobMigrateRequests.Add(int64(n))
	prometheusJobMigrateRequests.Add(float64(n))
}

func incJobUnscheduleRequests(n int) {
	expvarJobUnscheduleRequests.Add(int64(n))
	prometheusJobUnscheduleRequests.Add(float64(n))
}

func incTaskScheduleRequests(n int) {
	expvarTaskScheduleRequests.Add(int64(n))
	prometheusTaskScheduleRequests.Add(float64(n))
}

func incTaskUnscheduleRequests(n int) {
	expvarTaskUnscheduleRequests.Add(int64(n))
	prometheusTaskUnscheduleRequests.Add(float64(n))
}

func incContainersPlaced(n int) {
	expvarContainersPlaced.Add(int64(n))
	prometheusContainersPlaced.Add(float64(n))
}

func incAgentsLost(n int) {
	expvarAgentsLost.Add(int64(n))
	prometheusAgentsLost.Add(float64(n))
}

func incAgentConnectionsEstablished(n int) {
	expvarAgentConnectionsEstablished.Add(int64(n))
	prometheusAgentConnectionsEstablished.Add(float64(n))
}

func incAgentConnectionsInterrupted(n int) {
	expvarAgentConnectionsInterrupted.Add(int64(n))
	prometheusAgentConnectionsInterrupted.Add(float64(n))
}

func incContainerEventsReceived(n int) {
	expvarContainerEventsReceived.Add(int64(n))
	prometheusContainerEventsReceived.Add(float64(n))
}
