package scheduler

import "github.com/codegangsta/cli"

// Command is the scheduler subcommand.
var Command = cli.Command{
	Name:        "scheduler",
	Usage:       "Control a Harpoon agent",
	Description: "Interact with a Harpoon scheduler.",
	Subcommands: []cli.Command{},
	Flags:       []cli.Flag{},
	HideHelp:    true,
}
