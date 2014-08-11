# Container Supervision

- **Status**: PROPOSAL
- **Author**: @bernerdschaefer

Use named pipes and unix domain sockets to supervise spawned containers.

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

## Proposal

There will be two special files in a container's run directory:

  - `control`: a unix domain socket
  - `status`: the container's last status

`control` exposes a bidirectional message stream, where messages are encoded
according to the Server-Sent Events spec.

`status` is written by the container on startup (before listening) and shutdown
(before closing the socket).

### agent -> container events

* `stop` — initiate graceful shutdown; no event data supplied
* `kill` — initiate forceful shutdown; no event data supplied

### container -> agent events

* `status` — the container and its process' status, sent on a regular interval.
  The data section will be a JSON-encoded [Heartbeat](http://godoc.org/github.com/soundcloud/harpoon/harpoon-agent/lib#Heartbeat).

### Details

- When an agent starts a container, it will wait for `net.Dial` on `control` to
  not return `ENOENT`.
- If an agent receives an `ECONNREFUSED` error, then the container is dead.
- If an agent receives an `EOF` while reading or an `EPIPE` while writing, the
  container is dead.
- If an agent receives `ECONNREFUSED`, `EOF`, or `EPIPE`, the container's
  status file can be read to get the last state. If the container cannot be
  connected to but the status file says it is "UP", the container crashed in a
  truly exceptional way.

## Simulation

```
$ harpoon-container command &

$ socat UNIX-CONNECT:./control -
2014/08/11 16:39:06 socat[12156] E connect(3, LEN=11 AF=1 "./control", 11): No such file or directory

$ socat UNIX-CONNECT:control -
event: status
data: {"status": "UP", "container_process_status": {"up": true, "container_metrics": {...}}}

event: status
data: {"status": "UP", "container_process_status": {"up": true, "container_metrics": {...}}}

> event: stop
>

event: status
data: {"status": "DOWN", "container_process_status": {"up": false, "exited": true, "exit_status": 0}}

$ cat status
{"status": "DOWN", "container_process_status": {"up": false, "exited": true, "exit_status": 0}}

$ socat UNIX-CONNECT:./control -
2014/08/11 16:00:06 socat[11884] E connect(3, LEN=11 AF=1 "./control", 11): Connection refused
```
