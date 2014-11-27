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
		artifactFlag,
		portFlag,
		envFlag,
		workingDirFlag,
		execFlag,
		memFlag,
		cpuFlag,
		fdFlag,
		tmpFlag,
		volFlag,
		startupFlag,
		shutdownFlag,
		restartFlag,
	},
	HideHelp: true,
}

var artifactFlag = cli.StringFlag{
	Name:  "artifact_url",
	Value: "http://ent.int.s-cloud.net/iss/simpleweb.tar.gz",
	Usage: "artifact URL",
}

var defaultPortFlag = cli.StringSlice([]string{"HTTP:0"})
var portFlag = cli.StringSliceFlag{
	Name:  "port",
	Value: &defaultPortFlag,
	Usage: "NAME:PORT, port to allocate, e.g. HTTP:0 (autoassign), ROOTKIT:31337 [repeatable]",
}

var defaultEnvFlag = cli.StringSlice([]string{"MSG=Hello, 世界"})
var envFlag = cli.StringSliceFlag{
	Name:  "env",
	Value: &defaultEnvFlag,
	Usage: "KEY=VAL, environment to set [repeatable]",
}

var workingDirFlag = cli.StringFlag{
	Name:  "working_dir",
	Value: "/",
	Usage: "working dir in container, before executing command",
}

var execFlag = cli.StringFlag{
	Name:  "exec",
	Value: "./simpleweb -listen=$PORT_HTTP -message=$MSG",
	Usage: "command to execute, with spaces to separate arguments",
}

var memFlag = cli.IntFlag{
	Name:  "mem",
	Value: 64,
	Usage: "memory limit (MB)",
}

var cpuFlag = cli.Float64Flag{
	Name:  "cpu",
	Value: 0.1,
	Usage: "CPU limit (fractional CPUs)",
}

var fdFlag = cli.IntFlag{
	Name:  "fd",
	Value: 32768,
	Usage: "file descriptor limit (ulimit -n)",
}

var tmpFlag = cli.StringSliceFlag{
	Name:  "tmp",
	Value: &cli.StringSlice{},
	Usage: "PATH:SIZE, temp mount to create, e.g. /tmp:-1 (unlimited), /var/scratch:1024 [repeatable]",
}

var volFlag = cli.StringSliceFlag{
	Name:  "vol",
	Value: &cli.StringSlice{},
	Usage: "DST:SRC, volume to mount, e.g. /metrics:/data/prometheus/apiv2 [repeatable]",
}

var startupFlag = cli.DurationFlag{
	Name:  "startup",
	Value: 3 * time.Second,
	Usage: "maximum time for container to start up",
}

var shutdownFlag = cli.DurationFlag{
	Name:  "shutdown",
	Value: 3 * time.Second,
	Usage: "maximum time for container to shut down",
}

var restartFlag = cli.StringFlag{
	Name:  "restart",
	Value: agent.OnFailureRestart,
	Usage: fmt.Sprintf("restart policy: %s, %s, %s", agent.NoRestart, agent.OnFailureRestart, agent.AlwaysRestart),
}

func configAction(c *cli.Context) {
	ports := map[string]uint16{}
	for _, s := range portFlag.Value.Value() {
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
	for _, s := range envFlag.Value.Value() {
		t := strings.SplitN(s, "=", 2)

		if t[0] == "" || t[1] == "" {
			log.Fatalf("-env %q: invalid", s)
		}

		env[t[0]] = t[1]
	}

	if fdFlag.Value < 0 {
		log.Fatalf("-fd %d: must be >= 0", fdFlag.Value)
	}

	tmp := map[string]int{}
	for _, s := range tmpFlag.Value.Value() {
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
	for _, s := range volFlag.Value.Value() {
		t := strings.SplitN(s, ":", 2)

		if t[0] == "" || t[1] == "" {
			log.Fatalf("-vol %q: invalid", s)
		}

		volumes[t[0]] = t[1]
	}

	cfg := agent.ContainerConfig{
		ArtifactURL: artifactFlag.Value,
		Ports:       ports,
		Env:         env,
		Command: agent.Command{
			WorkingDir: workingDirFlag.Value,
			Exec:       strings.Split(execFlag.Value, " "), // TODO(pb): maybe something nicer
		},
		Resources: agent.Resources{
			Mem: uint64(memFlag.Value),
			CPU: cpuFlag.Value,
			FD:  uint64(fdFlag.Value),
		},
		Storage: agent.Storage{
			Tmp:     tmp,
			Volumes: volumes,
		},
		Grace: agent.Grace{
			Startup:  agent.JSONDuration{Duration: startupFlag.Value},
			Shutdown: agent.JSONDuration{Duration: shutdownFlag.Value},
		},
		Restart: agent.Restart(restartFlag.Value),
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
