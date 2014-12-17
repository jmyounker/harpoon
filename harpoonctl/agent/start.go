package agent

import (
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var startCommand = cli.Command{
	Name:        "start",
	Usage:       "Start an allocated container",
	Description: "start <ID>",
	Action:      startAction,
	Flags:       []cli.Flag{waitTimeoutFlag},
}

const startUsage = "start <ID>"

func startAction(c *cli.Context) {
	var (
		id          = c.Args().First()
		waitTimeout = c.Duration("wait-timeout")
	)

	if id == "" {
		log.Fatalf("usage: %s", startUsage)
	}

	start(id, waitTimeout)
}

func start(id string, waitTimeout time.Duration) {
	var (
		wg = sync.WaitGroup{}
		ok = int32(0)
	)

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

			statuses := map[agent.ContainerStatus]struct{}{
				agent.ContainerStatusRunning: struct{}{},
			}
			waitc := c.Wait(id, statuses, waitTimeout)

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

			log.Printf("%s: start %s requested", u.Host, id)

			result := <-waitc
			if result.Err != nil {
				log.Errorf("%s: while waiting for start: %s", u.Host, result.Err)
				return
			}

			log.Printf("%s: %s %s", u.Host, id, result.Status)

			atomic.AddInt32(&ok, 1)
		}(u)
	}

	wg.Wait()

	log.Printf("%d successfully started", ok)
}
