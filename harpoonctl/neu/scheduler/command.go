package scheduler

import "github.com/codegangsta/cli"

// Command is the scheduler subcommand.
var Command = cli.Command{
	Name:            "scheduler",
	ShortName:       "s",
	Usage:           "Control a Harpoon scheduler",
	Description:     "Interact with a Harpoon scheduler directly.",
	BashComplete:    func(c *cli.Context) {},
	Before:          func(c *cli.Context) error { return nil },
	Action:          nil,
	Subcommands:     []cli.Command{},
	Flags:           []cli.Flag{},
	SkipFlagParsing: false,
	HideHelp:        false,
}
