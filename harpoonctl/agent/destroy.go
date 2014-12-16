package agent

import (
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var destroyCommand = cli.Command{
	Name:        "destroy",
	Usage:       "Destroy a stopped container",
	Description: destroyUsage,
	Action:      destroyAction,
}

const destroyUsage = "destroy <ID>"

func destroyAction(c *cli.Context) {
	id := c.Args().First()
	if id == "" {
		log.Fatalf("usage: %s", destroyUsage)
	}

	destroy(id)
}

func destroy(id string) {
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

			if err := c.Destroy(id); err != nil {
				log.Verbosef("%s: %s", u.Host, err)
				return
			}

			log.Printf("%s: destroy %s OK", u.Host, id)

			atomic.AddInt32(&ok, 1)
		}(u)
	}

	wg.Wait()

	log.Printf("destroy %s complete, %d successfully destroyed", id, ok)
}
