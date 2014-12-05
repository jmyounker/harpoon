package agent

import (
	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var runCommand = cli.Command{
	Name:        "run",
	Usage:       "run <config.json> <id>",
	Description: "Creates (allocates) and starts a container.",
	Action:      runAction,
	Flags:       []cli.Flag{timeoutFlag},
}

func runAction(c *cli.Context) {
	if len(c.Args()) != 2 {
		log.Fatalf("usage: run <config.json> <id>")
	}

	var (
		filename = c.Args()[0]
		id       = c.Args()[1]
		timeout  = c.Duration("timeout")
	)

	create(filename, id, timeout, true)
}
