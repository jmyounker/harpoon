# harpoon-supervisor

`harpoon-supervisor` starts and monitors a container.

## Starting

`harpoon-supervisor` must be executed from a directory containing the following
two files:

  - `agent.json`-a harpoon agent file: a json serialized agent.ContainerConfig object
  - `rootfs`—the container's root filesystem (directory or symlink)

The supervisor has two mandatory arguments, `--hostname` and `--ID`.  The hostname
should be the hostname the supervisor thinks its running in, and ID should is an
arbitrary string that uniquely identifies the container on this machine.

These mandatory arguments are followed by the option '--' and everything after this
options is interpreted as the command to be executed within the container.

## Signals

If `harpoon-supervisor` receives a TERM or INT signal, it will initiate a
graceful shutdown: the container will be sent a TERM signal, and the supervisor
process will exit after the container exits. Sending a second TERM or INT
signal will cause the container to be sent a KILL signal.

## Control

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
$ harpoon-supervisor --hostname your.host.name --ID somejobid -- command &

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
