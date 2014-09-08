# harpoon-scheduler

A general-purpose scheduling component in the harpoon ecosystem.

## Architecture

```
+------------------+
|       API        |
+------------------+
  |              ^
  v              |
+----------+     |
| Registry |     |
+----------+     |
  |              |
  v              |
+-------------+  |
| Transformer |  |
+-------------+  |
  ^ |            |
  | v            |
+------------------+
|     Shepherd     |
+------------------+
        | ^
        v |
 +----------------+
 | State machines |
 +----------------+
```

### Scheduler

The scheduler receives user domain requests, executes the scheduling algorithm
(when necessary), and writes actionable information into the registry. User
domain requests may include

1. Schedule (start) a job
2. Migrate a scheduled (running) job to a new configuration
3. Unschedule (stop) a job

### API

The API converts REST-y HTTP actions into mutations on the registry. It also
provides a view on the state of the scheduling domain, by subscribing to
actual-state updates from the shepherd.

### Registry

The registry is a plain data store. It has two roles. It's a job scheduler,
meaning it receives requests to schedule and unschedule jobs. It's also a
desired-state broadcaster, which means other components can subscribe to it,
to receive the current desired-state of the scheduling domain.

The registry is a distinct component between the scheduler and transformer
for two reasons:

1. We can easily serialize and persist its state. (Corollary: the scheduler
   and transformer may therefore be mostly stateless.)
2. It decouples intent from action, which allows us to more easily reason
   about each individual part of the scheduling workflow.

Note that the registry represents only what the scheduler is responsible for,
and not necessarily the complete state of the candidate agents.

### Transformer

The transformer is an intermediary between our desired/logical state
(represented by the registry) and the actual/physical state (represented by
the shepherd). The transformer subscribes to updates from the registry and the
shepherd, and whenever anything changes, it determines if it needs to emit
task-schedule events to the shepherd.

### Shepherd

The shepherd simply provides a single interface point for all state machines.
It's an actual-state broadcaster, so interested components can subscribe and
get an up-to-date view of the scheduling domain. It also accepts task-schedule
commands, from the transformer.

### State machines

A state machine represents a remote harpoon-agent. It contains all the code to
make and maintain an event stream connection. State machines are created and
updated by an agent discovery component, which (right now) only has a static
list implementation.
