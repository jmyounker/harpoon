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

var stopCommand = cli.Command{
	Name:        "stop",
	Usage:       "Stop a running container",
	Description: stopUsage,
	Action:      stopAction,
	Flags:       []cli.Flag{waitTimeoutFlag},
}

const stopUsage = "stop <ID>"

var waitTimeoutFlag = cli.DurationFlag{
	Name:  "wait-timeout",
	Value: 10 * time.Second,
	Usage: "Max time to wait for container to change status",
}

func stopAction(c *cli.Context) {
	id := c.Args().First()
	if id == "" {
		log.Fatalf("usage: %s", stopUsage)
	}

	var (
		waitTimeout = c.Duration("wait-timeout")
	)

	stop(id, waitTimeout)
}

func stop(id string, waitTimeout time.Duration) {
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
				agent.ContainerStatusFinished: struct{}{},
			}
			waitc := c.Wait(id, statuses, waitTimeout)

			if err := c.Stop(id); err != nil {
				log.Verbosef("%s: %s", u.Host, err)
				return
			}

			log.Printf("%s: stop %s requested", u.Host, id)

			result := <-waitc
			if result.Err != nil {
				log.Errorf("%s: while waiting for stop: %s", u.Host, result.Err)
				return
			}

			log.Printf("%s: %s %s", u.Host, id, result.Status)

			atomic.AddInt32(&ok, 1)
		}(u)
	}

	wg.Wait()

	log.Printf("%d successfully stopped", ok)
}
