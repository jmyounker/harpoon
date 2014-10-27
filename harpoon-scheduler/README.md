# harpoon-scheduler

A general-purpose, monolithic scheduler for the Harpoon platform.

## Architecture

```
+-----+   +----------+   +-------------+   +-------+    +------+
| API |-->| Registry |-->| Transformer |<--| Proxy |<-->| Repr |<-----> Agent
|     |   +----------+   +-------------+   |       |    +------+
|     |                         '--------->|       |    +------+
|     |<-----------------------------------|       |<-->| Repr |<-----> Agent
+-----+                                    +-------+    +------+
```

### API

[Package api](https://github.com/soundcloud/harpoon/tree/master/harpoon-scheduler/api)
implements the public-facing scheduler HTTP API. All

- `POST /api/v0/schedule` with JSON-encoded [JobConfig][] in the request body.
   Writes the job to the registry, and returns HTTP 202 Accepted.

- `POST /api/v0/unschedule` with JSON-encoded [JobConfig][] in the request body.
  Removes the job from the registry, and returns HTTP 202 Accepted.

[JobConfig]: https://godoc.org/github.com/soundcloud/harpoon/harpoon-configstore/lib#JobConfig

### Registry

[Package registry](https://github.com/soundcloud/harpoon/tree/master/harpoon-scheduler/registry)
implements a simple, persistent data store. The registry accepts writes from
clients, and emits state changes to subscribers. The registry represents the
desired state of the scheduling domain.

The registry persists unassigned jobs. The transformer is responsible for
invoking the scheduling algorithm, and mapping tasks to agents.

### Transformer

[Package xf](https://github.com/soundcloud/harpoon/tree/master/harpoon-scheduler/xf)
implements the transformer, a component that subscribes to updates from the
desired state (registry) and actual state (proxy) of the scheduling domain.
The transformer continuously diffs desired v. actual, invokes the scheduling
algorithm to place unassigned containers, and emits mutations to agents, via
the proxy.

### Scheduling algorithms

[Package algo](https://github.com/soundcloud/harpoon/tree/master/harpoon-scheduler/algo)
implements scheduling algorithms, which are modeled as functions with perfect
knowledge of the scheduling domain. Scheduling algorithms are parameterized on
the set of containers that should be scheduled (including their resource
requirements), and all agents available in the scheduling domain (including
their resource availability). Different scheduling algorithms can implement
different biases and preferences.

### Proxy

[Package reprproxy](https://github.com/soundcloud/harpoon/tree/master/harpoon-scheduler/reprproxy)
implements a proxy (aggregator) for remote agent representations. It presents
a unified interface for other components to interact with any agent in the
scheduling domain.

The reprproxy is a bit complex. Please see the package README for more info.

### Representation

[Package agentrepr](https://github.com/soundcloud/harpoon/tree/master/harpoon-scheduler/agentrepr)
implements a local representation of a remote Harpoon agent. It's modeled as a
[CQRS](http://martinfowler.com/bliki/CQRS.html)
system. The repr connects to the remote agent's event stream, and treats that
stream as the sole, authoritative source of information about the agent. Each
incoming event acts as a transition applied to a per-container state machine.
Commands, i.e. schedule and unschedule requests, are fired asynchronously
toward the agent. Success or failure is an emergent property of the stream.

The agentrepr is complex. Please see the package README for more info.

### Misc

[Package metrics](https://github.com/soundcloud/harpoon/tree/master/harpoon-scheduler/metrics)
implements instrumentation for the scheduler, exposed via
[expvar](http://golang.org/pkg/expvar)
and
[Prometheus](http://github.com/prometheus)
endpoints.

[Package xtime](https://github.com/soundcloud/harpoon/tree/master/harpoon-scheduler/xtime)
wraps some
[time](http://golang.org/pkg/time)
functions, to make the components easier to test in timing-based scenarios.
