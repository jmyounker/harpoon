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
	Flags:       []cli.Flag{waitTimeoutFlag, downloadTimeoutFlag},
}

const replaceUsage = "replace <old ID> <new manifest.json>"

func replaceAction(c *cli.Context) {
	if len(c.Args()) != 2 {
		log.Fatalf("usage: %s", replaceUsage)
	}

	var (
		id              = c.Args()[0]
		filename        = c.Args()[1]
		waitTimeout     = c.Duration("wait-timeout")
		downloadTimeout = c.Duration("download-timeout")
	)

	// Most-naïve implementation.

	log.Verbosef("stopping existing %s, if any", id)
	stop(id, waitTimeout)

	log.Verbosef("destroying existing %s, if any", id)
	destroy(id, waitTimeout)

	log.Verbosef("starting %s with %s", id, filename)
	create(id, filename, downloadTimeout, true)

	log.Printf("successfully replaced %s", id)
}
