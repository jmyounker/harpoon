package agent

import (
	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var runCommand = cli.Command{
	Name:        "run",
	Usage:       "Create and start a container",
	Description: runUsage,
	Action:      runAction,
	Flags:       []cli.Flag{downloadTimeoutFlag},
}

const runUsage = "run <ID> <manifest.json>"

func runAction(c *cli.Context) {
	if len(c.Args()) != 2 {
		log.Fatalf("usage: %s", runUsage)
	}

	var (
		id       = c.Args()[0]
		filename = c.Args()[1]
		timeout  = c.Duration("timeout")
	)

	create(id, filename, timeout, true)
}
