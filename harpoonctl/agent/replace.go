package agent

import (
	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var replaceCommand = cli.Command{
	Name:        "replace",
	Usage:       "Create-or-update and start a container",
	Description: replaceUsage,
	Action:      replaceAction,
	Flags:       []cli.Flag{timeoutFlag},
}

const replaceUsage = "replace <old ID> <new manifest.json>"

func replaceAction(c *cli.Context) {
	if len(c.Args()) != 2 {
		log.Fatalf("usage: %s", replaceUsage)
	}

	var (
		id       = c.Args()[0]
		filename = c.Args()[1]
		timeout  = c.Duration("timeout")
	)

	// Most-naïve implementation.

	stop(id)
	destroy(id)
	create(filename, id, timeout, true)

	log.Printf("successfully replaced %s", id)
}
