package agent

import (
	"log"

	"github.com/codegangsta/cli"
)

var resourcesCommand = cli.Command{
	Name:        "resources",
	ShortName:   "r",
	Usage:       "Print agent resources",
	Description: "Display instantaneous total and reserved CPU, memory, etc. resources of agent(s).",
	Action:      resourcesAction,
	Flags:       []cli.Flag{longFlag},
}

var longFlag = cli.BoolFlag{
	Name:  "l, long",
	Usage: "Long (verbose) output",
}

func resourcesAction(c *cli.Context) {
	log.Printf("resources output")
	if c.Bool("long") {
		log.Printf("resources (long) output")
	}
	log.Printf("GlobalStringSlice endpoint %v", c.GlobalStringSlice("endpoint"))
	log.Printf("StringSlice endpoint %v", c.StringSlice("endpoint"))
	log.Printf("GlobalStringSlice e %v", c.GlobalStringSlice("e"))
	log.Printf("StringSlice e %v", c.StringSlice("e"))
}
