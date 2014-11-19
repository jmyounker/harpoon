package agent

import (
	"fmt"
	"net/url"
	"os"
	"sync"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var logCommand = cli.Command{
	Name:        "log",
	Usage:       "log <id>",
	Description: "Streams logs from a running container.",
	Action:      logAction,
	Flags:       []cli.Flag{historyFlag},
}

var historyFlag = cli.IntFlag{
	Name:  "n, history",
	Value: 0,
	Usage: "historical log lines to include",
}

func logAction(c *cli.Context) {
	var (
		id = c.Args().First()
		n  = c.Int("history")
		ch = make(chan (<-chan string), len(endpoints))
		wg = sync.WaitGroup{}
	)

	if id == "" {
		log.Fatalf("usage: log <id>")
	}

	// Non-graceful termination, as the agent.Log's Stopper is nonresponsive
	// if the container doesn't output a steady stream of log lines. We should
	// probably fix that.

	for _, u := range endpoints {
		go func(u *url.URL) {
			var c <-chan string
			defer func() { ch <- c }()

			client, err := agent.NewClient(u.String())
			if err != nil {
				log.Warnf("%s: %s", u.Host, err)
				return
			}

			log.Verbosef("%s: checking %s...", u.Host, id)

			c, _, err = client.Log(id, n)
			if err == agent.ErrContainerNotExist {
				log.Verbosef("%s: %s doesn't exist", u.Host, id)
				return
			} else if err != nil {
				log.Warnf("%s: %s", u.Host, err)
				return
			}

			log.Verbosef("%s: %s found", u.Host, id)
		}(u)
	}

	for i := 0; i < cap(ch); i++ {
		c := <-ch
		if c == nil {
			continue
		}

		wg.Add(1)

		go func() {
			for line := range c {
				fmt.Fprintf(os.Stdout, line)
			}
		}()
	}

	wg.Wait()
}
