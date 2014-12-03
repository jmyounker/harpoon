package agent

import (
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var startCommand = cli.Command{
	Name:        "start",
	Usage:       "start <id>",
	Description: "Starts a created, finished, or failed container.",
	Action:      startAction,
}

func startAction(c *cli.Context) {
	var (
		id = c.Args().First()
		wg = sync.WaitGroup{}
		ok = int32(0)
	)

	if id == "" {
		log.Fatalf("usage: start <id>")
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

			m, err := c.Containers()
			if err != nil {
				log.Warnf("%s: %s", u.Host, err)
				return
			}

			if _, ok := m[id]; !ok {
				log.Verbosef("%s: doesn't have %s", u.Host, id)
				return
			}

			switch m[id].ContainerStatus {
			case agent.ContainerStatusCreated, agent.ContainerStatusFinished, agent.ContainerStatusFailed:
			case agent.ContainerStatusRunning:
				log.Printf("%s: %s already running", u.Host, id)
				return
			}

			if err := c.Start(id); err != nil {
				log.Warnf("%s: %s", u.Host, err)
				return
			}

			log.Printf("%s: start %s OK", u.Host, id)

			atomic.AddInt32(&ok, 1)
		}(u)
	}

	wg.Wait()

	log.Printf("start %s complete, %d successfully started", id, ok)
}
