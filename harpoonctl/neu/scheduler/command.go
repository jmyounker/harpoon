package scheduler

import "github.com/codegangsta/cli"

var Verbosef = func(string, ...interface{}) {}

// Command is the scheduler subcommand.
var Command = cli.Command{
	Name:        "scheduler",
	Usage:       "Control a Harpoon scheduler",
	Description: "Interact with a Harpoon scheduler directly.",
	Subcommands: []cli.Command{},
	Flags:       []cli.Flag{},
	HideHelp:    true,
}
