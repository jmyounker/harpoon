# Container Supervision

- **Status**: IMPLEMENTED
- **Author**: @bernerdschaefer

Use unix domain socket to supervise spawned containers.

## Background

The techniques harpoon uses to translate requests to an agent's HTTP API into
actions in a container grew out of an existing (internal) container system in
use at SoundCloud.

Specifically:

  1. The agent does not itself execute user commands in a container, but spawns
     separate process to do this.
  2. The agent does not rely on process ownership to control this process.
  3. The container uses a heartbeat protocol to synchronize its state with the
     desired state stored in an agent.

This model was chosen primarily to separate the lifecycle of the agent from the
containers it runs.

The model outlined above has worked well, but is not without drawbacks:

  - An agent cannot distinguish missed heartbeats from a crashed container.
  - It is difficult to reason about the agent and container's state machines.
  - Features like restarting or sending signals to a container are not easily
    implemented as state transitions.

The first drawback is indeed the most critical. If no heartbeats are received
by an agent for some time, the container must be declared to be in an undefined
state.

---

## Changelog

The implementation of the supervision design reflected here differs from the
[initial proposal][1137bd] in a few ways:

  - "harpoon-container" has been renamed "harpoon-supervisor"
  - the state data type has been restructured
  - the special `state` file has been dropped

Removing the state file simplifies the implementation, and reduces the surface
area for errors. The same guarantees the state file provided are accomplished
by requiring an explicit 'exit' command to shut down the supervisor, which
functions as an ack of the final state.

[1137bd]: https://github.com/soundcloud/harpoon/blob/1137bd97f40e56272689ca57a5a81133ee32195b/doc/design/container-supervision.md

---

## harpoon-supervisor

`harpoon-supervisor` creates a unix socket called `control` in the current
directory. This socket can be used to collect state information about the
container, as well as control it (e.g., shut down).

### State

When a process connects to the `control` socket, it will receive a stream of
state events. The events will be encoded as Server-Sent Events, with an event
type of `state` and the data the JSON encoding of ContainerProcessState.

The current state will be sent immediately on connecting, and subsequent states
will be sent 1) when metrics are collected, and 2) when the process state
changes.

### Commands

Commands can be also sent to the supervisor over the `control` socket. The
commands must be encoded as Server-Sent Events.

Currently supported commands are:

  * `stop` — initiate graceful shutdown; no event data supplied
  * `kill` — initiate forceful shutdown; no event data supplied
  * `exit` — terminate supervisor; no event data supplied; noop if container
    process is not already stopped or killed.

### Simulation

```
$ harpoon-supervisor command &

$ socat UNIX-CONNECT:./control - &

event: state
data: {"up": true, "restarting": true, "restarts": 0, "ooms": 0, "container_metrics": {...}}

event: state
data: {"up": true, "restarting": true, "restarts": 0, "ooms": 0, "container_metrics": {...}}

$ printf "event: stop\n\n" | socat UNIX-CONNECT:./control -

event: state
data: {"up": true, "restarting": false, "restarts": 0, "ooms": 0, "container_metrics": {...}}

$ printf "event: kill\n\n" | socat UNIX-CONNECT:./control -

event: state
data: {"up": false, "restarting": false, "container_exit_status": {"signaled": true, "signal": 9}, "restarts": 0, "ooms": 0, "container_metrics": {...}}

$ printf "event: exit\n\n" | socat UNIX-CONNECT:./control -

```
