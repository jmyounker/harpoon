package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoonctl/log"
)

// Command is the config subcommand.
var Command = cli.Command{
	Name:        "config",
	Usage:       "Create container config",
	Description: "Create a container configuration file.",
	Action:      configAction,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "artifact_url",
			Value: "http://ent.int.s-cloud.net/iss/simpleweb.tar.gz",
			Usage: "artifact URL",
		},
		cli.StringSliceFlag{
			Name:  "port",
			Value: &defaultPortFlag,
			Usage: "port to allocate, NAME:PORT, e.g. HTTP:0 (autoassign), ROOTKIT:31337 [repeatable]",
		},
		cli.StringSliceFlag{
			Name:  "env",
			Value: &defaultEnvFlag,
			Usage: "env var to set, KEY=VAL [repeatable]",
		},
		cli.StringFlag{
			Name:  "working_dir",
			Value: "/",
			Usage: "working dir in container, before executing command",
		},
		cli.StringFlag{
			Name:  "exec",
			Value: "./simpleweb -listen=$PORT_HTTP -message=$MSG",
			Usage: "command to execute, with spaces to separate arguments",
		},
		cli.IntFlag{
			Name:  "mem",
			Value: 64,
			Usage: "memory limit (MB)",
		},
		cli.Float64Flag{
			Name:  "cpu",
			Value: 0.1,
			Usage: "CPU limit (fractional CPUs)",
		},
		cli.IntFlag{
			Name:  "fd",
			Value: 32768,
			Usage: "file descriptor limit (ulimit -n)",
		},
		cli.StringSliceFlag{
			Name:  "tmp",
			Value: &cli.StringSlice{},
			Usage: "tmp mount to create, PATH:SIZE, e.g. /tmp:-1 (unlimited), /var/scratch:1024 [repeatable]",
		},
		cli.StringSliceFlag{
			Name:  "vol",
			Value: &cli.StringSlice{},
			Usage: "host volume to mount, DST:SRC, e.g. /metrics:/data/prometheus/apiv2 [repeatable]",
		},
		cli.DurationFlag{
			Name:  "startup",
			Value: 3 * time.Second,
			Usage: "maximum time for container to start up",
		},
		cli.DurationFlag{
			Name:  "shutdown",
			Value: 3 * time.Second,
			Usage: "maximum time for container to shut down",
		},
		cli.StringFlag{
			Name:  "restart",
			Value: agent.OnFailureRestart,
			Usage: fmt.Sprintf("restart policy: %s, %s, %s", agent.NoRestart, agent.OnFailureRestart, agent.AlwaysRestart),
		},
	},
	HideHelp: true,
}

var (
	defaultPortFlag = cli.StringSlice([]string{"HTTP:0"})
	defaultEnvFlag  = cli.StringSlice([]string{"MSG=Hello, 世界"})
)

func configAction(c *cli.Context) {
	ports := map[string]uint16{}
	for _, s := range c.StringSlice("port") {
		t := strings.SplitN(s, ":", 2)

		if t[0] == "" || t[1] == "" {
			log.Fatalf("-port %q: invalid", s)
		}

		i, err := strconv.ParseUint(t[1], 10, 16)
		if err != nil {
			log.Fatalf("-port %q: %s", s, err)
		}

		ports[t[0]] = uint16(i)
	}

	env := map[string]string{}
	for _, s := range c.StringSlice("env") {
		t := strings.SplitN(s, "=", 2)

		if t[0] == "" || t[1] == "" {
			log.Fatalf("-env %q: invalid", s)
		}

		env[t[0]] = t[1]
	}

	if mem := c.Int("mem"); mem <= 0 {
		log.Fatalf("-mem %d: must be > 0", mem)
	}

	if cpu := c.Float64("cpu"); cpu <= 0.0 {
		log.Fatalf("-cpu %.2f: must be > 0.0", cpu)
	}

	if fd := c.Int("fd"); fd < 0 {
		log.Fatalf("-fd %d: must be >= 0", fd)
	}

	tmp := map[string]int{}
	for _, s := range c.StringSlice("tmp") {
		t := strings.SplitN(s, ":", 2)

		if t[0] == "" || t[1] == "" {
			log.Fatalf("-tmp %q: invalid", s)
		}

		i, err := strconv.ParseInt(t[1], 10, 64)
		if err != nil {
			log.Fatalf("-tmp %q: %s", s, err)
		}

		tmp[t[0]] = int(i)
	}

	volumes := map[string]string{}
	for _, s := range c.StringSlice("vol") {
		t := strings.SplitN(s, ":", 2)

		if t[0] == "" || t[1] == "" {
			log.Fatalf("-vol %q: invalid", s)
		}

		volumes[t[0]] = t[1]
	}

	cfg := agent.ContainerConfig{
		ArtifactURL: c.String("artifact_url"),
		Ports:       ports,
		Env:         env,
		Command: agent.Command{
			WorkingDir: c.String("working_dir"),
			Exec:       strings.Split(c.String("exec"), " "), // TODO(pb): maybe something nicer
		},
		Resources: agent.Resources{
			Mem: uint64(c.Int("mem")),
			CPU: c.Float64("cpu"),
			FD:  uint64(c.Int("fd")),
		},
		Storage: agent.Storage{
			Tmp:     tmp,
			Volumes: volumes,
		},
		Grace: agent.Grace{
			Startup:  agent.JSONDuration{Duration: c.Duration("startup")},
			Shutdown: agent.JSONDuration{Duration: c.Duration("shutdown")},
		},
		Restart: agent.Restart(c.String("restart")),
	}

	if err := cfg.Valid(); err != nil {
		log.Fatalf("%s", err)
	}

	buf, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		log.Fatalf("%v", err)
	}

	fmt.Fprintf(os.Stdout, "%s\n", buf)
}
