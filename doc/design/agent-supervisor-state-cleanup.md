# Agent And Supervisor State Machines

- **Status**: WIP
- **Author**: @jmy

Use unix domain socket to supervise spawned containers.

## Background

The states for the agent and supervisor have never been explicitly mapped.  We've also added
a number of features (e.g. restart policies) to the agent-supervisor system since initial
implementation. As we've been pushing harpoon towards production we've been running into
bugs which seem to be related to the agent and supervisor states being inadequately
addressed.  (E.g. the supervisor dies and the agent looses track of its state; you stop
the container explicitly but the restart policy brings it back to life.) 

Addressing these seems to indicate adding more variables, or duplicating code, etc.
I've mapped out the [supervisor's current implicit state diagram](https://cloud.githubusercontent.com/assets/882634/5411707/d04ae674-8203-11e4-9ea8-d7f352b547ed.png),
and it's a mess.  Rather than splicing in more meta-states to address the collection
of oddnesses that we've been encountering, it looks more fruitful to clean up both the
supervisor and agent state machines, and to make them both explicit, resulting in this
[simplified state machine diagram](https://cloud.githubusercontent.com/assets/882634/5411708/d9d7399a-8203-11e4-984f-006ab438b1df.png).

There is one feature set that I'm removing as part of this work.  Orphane supervisors will
no longer restart containers on their own, as this logic is moving into the agent.

This has several impacts, the most obvious being that if the agent goes down, you can no
longer count on containers to continue running indefinitely.  It also means that OOM and restart
tracking become the agent's responsibility.

Moving the restart logic from supervisor to agent has these advantages:

  * The supervisor state machine becomes much simpler and clearer.  It transitions through a small number of
    states, and currently problematic transitions are eliminated. There are no loops between state transitions.
    
  * The supervisor's lifecycle closely mimics the container's.
  
  * The agent understands the origin of commands, so it can apply restart policies based on history.  Moving
    restart policy into the agent only adds one additional state to the agent's state machine. (The **Restarting**
    state in the [agent state diagram](https://cloud.githubusercontent.com/assets/882634/5411702/b52c7678-8203-11e4-85cc-a8e61bf00e25.png).)
  
  * Simpler interactions between agent and supervisor states.

In the future we may move certain classes of restarts back into the supervisor once we've achieved stable
operations.

---

## Changelog


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


### States

  * `starting` - container is starting
  * `up` - conatiner is running
  * `stopping` - container is shutting down
  * `exit` - container has exited, and supervisor is shutting down

### Simulation

```
$ harpoon-supervisor command &

$ socat UNIX-CONNECT:./control - &

event: state
data: {"state": "starting", "container_metrics": {...}}

event: state
data: {"state": "up", "container_metrics": {...}}

$ printf "event: stop\n\n" | socat UNIX-CONNECT:./control -

event: state
data: {"state": "stopping", "container_metrics": {...}}

$ printf "event: kill\n\n" | socat UNIX-CONNECT:./control -

event: state
data: {"state": "exit", "container_exit_status": {"signaled": true, "signal": 9}, "container_metrics": {...}}

$ printf "event: exit\n\n" | socat UNIX-CONNECT:./control -

```
