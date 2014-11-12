package main

import (
	"net"
	"net/http"
	"os"
	"time"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoonctl/agent"
	"github.com/soundcloud/harpoon/harpoonctl/log"
	"github.com/soundcloud/harpoon/harpoonctl/scheduler"
)

// http://monkey.org/~marius/unix-tools-hints.html
// http://www.catb.org/~esr/writings/taouu/html/

func main() {
	a := &cli.App{
		Name:                 "harpoonctl",
		Usage:                "Interact with Harpoon platform components.",
		Version:              version(),
		Commands:             []cli.Command{agent.Command, scheduler.Command},
		Flags:                []cli.Flag{verboseFlag, timeoutFlag},
		EnableBashCompletion: false,
		HideHelp:             true,
		HideVersion:          true,
		Before:               before,
		Action:               cli.ShowAppHelp,
		CommandNotFound:      func(c *cli.Context, cmd string) { log.Warnf("%s: not found", cmd) },
		Compiled:             compileTime(),
		Author:               "Infrastructure Software and Services",
		Email:                "iss@soundcloud.com",
	}
	a.Run(os.Args)
}

var verboseFlag = cli.BoolFlag{
	Name:  "v, verbose",
	Usage: "print verbose output",
}

var timeoutFlag = cli.DurationFlag{
	Name:  "t, timeout",
	Value: 3 * time.Second,
	Usage: "HTTP connection timeout",
}

func before(c *cli.Context) error {
	if c.Bool("verbose") {
		log.Verbose = true
	}

	http.DefaultClient.Transport = &http.Transport{
		Dial: func(network, addr string) (net.Conn, error) {
			return net.DialTimeout(network, addr, c.Duration("timeout"))
		},
	}

	return nil
}

func compileTime() time.Time {
	info, err := os.Stat(os.Args[0])
	if err != nil {
		return time.Now()
	}
	return info.ModTime()
}
