# harpoonctl

```
USAGE: harpoonctl [GLOBAL OPTIONS...] <command> [<args>]

harpoonctl controls one or more harpoon agents without a central scheduler.

Commands default to communicating with a local harpoon agent, unless a default
cluster (~/.harpoonctl/cluster/default) is defined.

GLOBAL OPTIONS:
  -v,--version		print the version
  -h,--help		show this help

  -a,--agent HOST:PORT  agent address (repeatable, overrides -c)
  -c,--cluster NAME     read agent addresses from ~/.harpoonctl/cluster/NAME.

COMMANDS:
   ps		list containers
   run		create and start a new container
   status	return information about a container
   stop		stop a container
   start	start a (stopped) container
   destroy	destroy a (stopped) container
   logs		fetch the logs of one or more containers
   resources	list agents and their resources
   help, h	Shows a list of commands or help for one command
```

## Examples

```
> harpoonctl resources
AGENT           MEM   RESERVED  CPU  RESERVED  VOLUMES
localhost:3333  1024  64        1    0.1       -

> harpoonctl run web.config.json
AGENT           INSTANCE            COMMAND  PORTS       STATUS   CREATED
localhost:3333  rocket:14816f3256a  ./web    http=31010  running  now

> harpoonctl ps
AGENT           INSTANCE            COMMAND  PORTS       STATUS   CREATED
localhost:3333  rocket:14816f3256a  ./web    http=31010  running  2 minutes ago
localhost:3333  rocket:14816f79af0  ./web    http=31009  running  20 minutes ago

> harpoonctl resources
AGENT           MEM   RESERVED  CPU  RESERVED  VOLUMES
localhost:3333  1024  128       1    0.2       -

> harpoonctl stop rocket:14816f3256a
AGENT           INSTANCE            COMMAND  PORTS       STATUS
localhost:3333  rocket:14816f3256a  ./web    http=31010  stopped

> harpoonctl destroy rocket:14816f3256a
AGENT           INSTANCE            COMMAND  PORTS       STATUS
localhost:3333  rocket:14816f3256a  ./web    http=31010  destroyed
```

### Clusters

Passing the addresses of many agents for each invocation would be cumbersome,
so `harpoonctl` also supports a `cluster` flag for talking to a set of agents:

```
cat <<-EOF > ~/.harpoonctl/cluster/memcached
10.70.26.77:3333
10.70.26.78:3333
EOF

> harpoonctl -c memcached run memcached.config.json
   AGENT             MEM    RESERVED  CPU  RESERVED  VOLUMES
1) 10.70.26.78:3333  12228  0         12   0.0       -
2) 10.70.26.77:3333  12228  4096      12   0.5       -

Select an agent [default: 1]: 2
AGENT             INSTANCE               COMMAND        PORTS       STATUS
10.70.26.78:3333  memcached:14816f79af0  ./memcached    tcp=11211  running

> harpoonctl -c memcached ps # list containers running in the memcached cluster
AGENT             INSTANCE               COMMAND        PORTS      STATUS
10.70.26.77:3333  memcached:14816f3256a  ./memcached    tcp=11211  running
10.70.26.78:3333  memcached:14816f79af0  ./memcached    tcp=11211  running

> harpoonctl -c memcached status memcached:14816f79af0
# ...

> harpoonctl -c memcached resources
AGENT             MEM    RESERVED  CPU  RESERVED  VOLUMES
10.70.26.77:3333  12228  4096      12   0.5       -
10.70.26.78:3333  12228  4096      12   0.5       -
```
