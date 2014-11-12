package agent

import (
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoonctl/neu/log"
)

var destroyCommand = cli.Command{
	Name:        "destroy",
	Usage:       "destroy <id>",
	Description: "Destroys a stopped container.",
	Action:      destroyAction,
	Flags:       []cli.Flag{},
}

func destroyAction(c *cli.Context) {
	var (
		id = c.Args().First()
		wg = sync.WaitGroup{}
		ok = int32(0)
	)

	if id == "" {
		log.Fatalf("usage: destroy <id>")
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

			if err := c.Destroy(id); err != nil {
				log.Warnf("%s: %s", u.Host, err)
				return
			}

			log.Printf("%s: destroy %s OK", u.Host, id)

			atomic.AddInt32(&ok, 1)
		}(u)
	}

	wg.Wait()

	log.Printf("destroy %s complete, %d successfully destroyed", id, ok)
}
