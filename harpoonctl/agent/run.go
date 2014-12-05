package agent

import (
	"encoding/json"
	"os"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
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
		filename        = c.Args()[0]
		id              = c.Args()[1]
		downloadTimeout = c.Duration("download.timeout")
	)

	f, err := os.Open(filename)
	if err != nil {
		log.Fatalf("%s: %s", filename, err)
	}
	defer f.Close()

	var cfg agent.ContainerConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		log.Fatalf("%s: %s", filename, err)
	}

	if err := cfg.Valid(); err != nil {
		log.Fatalf("%s: %s", filename, err)
	}

	u := chooseEndpoint(c)

	client, err := agent.NewClient(u.String())
	if err != nil {
		log.Fatalf("%s: %s", u.Host, err)
	}

	// Start listening for client creation before we issue the Create call, otherwise there is a small window
	// in which we can loose responses from the client.
	wanted := map[agent.ContainerStatus]struct{}{
		agent.ContainerStatusCreated: struct{}{},
		agent.ContainerStatusDeleted: struct{}{},
	}
	wc := client.Wait(id, wanted, downloadTimeout)

	// Issue create
	if err := client.Put(id, cfg); err != nil {
		log.Fatalf("%s: %s", u.Host, err)
	}

	// Check results from create
	w := <-wc

	if w.Err != nil {
		log.Fatalf("%s: %s", u.Host, w.Err)
	}

	if w.Status == agent.ContainerStatusDeleted {
		log.Fatalf("%s: container creation failed", id)
	}

	// Create succeeded, so it's time start the process
	if err := client.Start(id); err != nil {
		log.Fatalf("%s: %s", u.Host, err)
	}

	log.Printf("%s: run %s (%s) OK", u.Host, id, filename)
}
