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

var destroyCommand = cli.Command{
	Name:        "destroy",
	Usage:       "Destroy a stopped container",
	Description: destroyUsage,
	Action:      destroyAction,
	Flags:       []cli.Flag{waitTimeoutFlag},
}

const destroyUsage = "destroy <ID>"

func destroyAction(c *cli.Context) {
	var (
		id          = c.Args().First()
		waitTimeout = c.Duration("wait-timeout")
	)

	if id == "" {
		log.Fatalf("usage: %s", destroyUsage)
	}

	destroy(id, waitTimeout)
}

func destroy(id string, waitTimeout time.Duration) {
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

			statuses := map[agent.ContainerStatus]struct{}{
				agent.ContainerStatusDeleted: struct{}{},
			}
			waitc := c.Wait(id, statuses, waitTimeout)

			if err := c.Destroy(id); err != nil {
				log.Verbosef("%s: %s", u.Host, err)
				return
			}

			log.Printf("%s: destroy %s requested", u.Host, id)

			result := <-waitc
			if result.Err != nil {
				log.Errorf("%s: while waiting for destroy: %s", u.Host, result.Err)
				return
			}

			log.Printf("%s: %s %s", u.Host, id, result.Status)

			atomic.AddInt32(&ok, 1)
		}(u)
	}

	wg.Wait()

	log.Printf("%d successfully destroyed", ok)
}
