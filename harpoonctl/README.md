# harpoonctl

```
NAME:
   harpoonctl - Interact with Harpoon platform components.

USAGE:
   harpoonctl [global options] command [command options] [arguments...]

VERSION:
   development

AUTHOR:
  Infrastructure Software and Services - <iss@soundcloud.com>

COMMANDS:
   agent  Control Harpoon agents
   scheduler  Control a Harpoon agent

GLOBAL OPTIONS:
   -v, --verbose  print verbose output
   -t, --timeout '3s' HTTP connection timeout
```

## harpoonctl agent

```
NAME:
   harpoonctl agent - Interact with Harpoon agents directly.

USAGE:
   harpoonctl agent command [command options] [arguments...]

COMMANDS:
   resources  Print agent host resources
   ps         Print instances on agent(s)
   dump       dump <id>
   log        log <id>
   create     create <config.json> <id>
   start      start <id>
   stop       stop <id>
   destroy    destroy <id>

OPTIONS:
   -e, --endpoint '-e option -e option'  agent endpoint(s) (repeatable, overrides --cluster)
   -c, --cluster 'default'               read agent endpoint(s) from $HOME/.harpoonctl/cluster/default
```

### Specifying endpoints

`harpoonctl agent` interacts with remote agent(s). Agents can be specified in
several ways. In order of increasing priority,

- `localhost:3333`
- All endpoints in `$HOME/.harpoonctl/cluster/default`, if it exists
- All endpoints in `$HOME/.harpoonctl/cluster/name`, if `--cluster name` is given
- All endpoints given explicitly by `--endpoint foo` (repeatable)

Cluster files live at `$HOME/.harpoonctl/cluster/name` and have one agent
endpoint per line.

```
$ cat > $HOME/.harpoonctl/cluster/test <<EOF
ip-10-70-11-20.eu-west.s-cloud.net:3333
ip-10-70-27-77.eu-west.s-cloud.net:3333
EOF

$ harpoonctl agent -c test resources
AGENT                                    CPU   TOTAL  MEM  TOTAL  VOLUMES
ip-10-70-11-20.eu-west.s-cloud.net:3333  0.00  1.00   0    1659   /data/foo, /data/bar
ip-10-70-27-77.eu-west.s-cloud.net:3333  0.10  1.00   64   1659   /data/prometheus
```

## harpoonctl scheduler

TODO