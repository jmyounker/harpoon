package main

import (
	"log"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/codegangsta/cli"
)

var (
	clusterPath    = filepath.Join(os.Getenv("HOME"), ".harpoonctl", "cluster")
	defaultCluster = filepath.Join(clusterPath, "default")
)

func main() {
	log.SetFlags(0)

	harpoonctl := &harpoonctl{
		Writer: tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0),
	}

	app := &cli.App{
		Name: "harpoonctl",
		Usage: `control one or more harpoon agents without a central scheduler

Commands default to communicating with a local harpoon agent, unless a default
cluster (` + defaultCluster + `) is defined.`,
		Version: "0.0.1",

		Action: cli.ShowAppHelp,

		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "c,cluster",
				Value: "",
				Usage: "read agent addresses from " + clusterPath + "/NAME",
			},

			cli.StringSliceFlag{
				Name:  "a,agent",
				Value: &cli.StringSlice{},
				Usage: "agent address (repeatable, overrides -c)",
			},
		},

		Before: harpoonctl.setAgents,

		Commands: []cli.Command{
			{
				Name:   "run",
				Usage:  "create and start a new container",
				Action: harpoonctl.run,
			},
			{
				Name:   "ps",
				Usage:  "list containers",
				Action: harpoonctl.ps,
			},
			{
				Name:   "status",
				Usage:  "return information about a container",
				Action: harpoonctl.status,
			},
			{
				Name:   "stop",
				Usage:  "stop a container",
				Action: harpoonctl.stop,
			},
			{
				Name:   "start",
				Usage:  "start a (stopped) container",
				Action: harpoonctl.start,
			},
			{
				Name:   "destroy",
				Usage:  "destroy a (stopped) container",
				Action: harpoonctl.destroy,
			},
			{
				Name:   "logs",
				Usage:  "fetch the logs of a container",
				Action: harpoonctl.logs,
			},
			{
				Name:   "resources",
				Usage:  "list agents and their resources",
				Action: harpoonctl.resources,
			},
		},
	}

	app.RunAndExitOnError()
}
