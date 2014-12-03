package agent

import (
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var stopCommand = cli.Command{
	Name:        "stop",
	Usage:       "stop <id>",
	Description: "Stops a running container.",
	Action:      stopAction,
}

func stopAction(c *cli.Context) {
	var (
		id = c.Args().First()
		wg = sync.WaitGroup{}
		ok = int32(0)
	)

	if id == "" {
		log.Fatalf("usage: stop <id>")
	}

	wg.Add(len(endpoints))

	for _, u := range endpoints {
		go func(u *url.URL) {
			defer wg.Done()

			c, err := agent.NewClient(u.String())
			if err != nil {
				log.Warnf("%s: %s", u.Host, err)
				return
			}

			if err := c.Stop(id); err != nil {
				log.Verbosef("%s: %s", u.Host, err)
				return
			}

			log.Printf("%s: stop %s OK", u.Host, id)

			atomic.AddInt32(&ok, 1)
		}(u)
	}

	wg.Wait()

	log.Printf("stop %s complete, %d successfully stopped", id, ok)
}
