package main

import (
	"log"
	"os"
	"time"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoonctl/neu/agent"
	"github.com/soundcloud/harpoon/harpoonctl/neu/scheduler"
)

func main() {
	log.SetFlags(0)

	a := &cli.App{
		Name:                 "harpoonctl",
		Usage:                "Control Harpoon components",
		Version:              version(),
		Commands:             []cli.Command{agent.Command, scheduler.Command},
		Flags:                []cli.Flag{},
		EnableBashCompletion: false,
		HideHelp:             false,
		Action:               cli.ShowAppHelp,
		CommandNotFound:      func(c *cli.Context, cmd string) { log.Printf("%s: not found", cmd) },
		Compiled:             compileTime(),
		Author:               "SoundCloud Infrastructure Software and Services",
		Email:                "iss@soundcloud.com",
	}

	a.Run(os.Args)
}

func compileTime() time.Time {
	info, err := os.Stat(os.Args[0])
	if err != nil {
		return time.Now()
	}
	return info.ModTime()
}
