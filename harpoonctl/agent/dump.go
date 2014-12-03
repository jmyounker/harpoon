package agent

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sync"
	"sync/atomic"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var dumpCommand = cli.Command{
	Name:        "dump",
	Usage:       "dump <id>",
	Description: "Dumps current state of a container to stdout as JSON.",
	Action:      dumpAction,
}

func dumpAction(c *cli.Context) {
	var (
		id = c.Args().First()
		wg = sync.WaitGroup{}
		ok = int32(0)
	)

	if id == "" {
		log.Fatalf("usage: dump <id>")
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

			ci, present := m[id]
			if !present {
				log.Verbosef("%s: %s not found", u.Host, id)
				return
			}

			buf, err := json.MarshalIndent(ci, "", "    ")
			if err != nil {
				log.Warnf("%s: %s", u.Host, err)
				return
			}

			fmt.Fprintf(os.Stdout, string(buf)+"\n")

			atomic.AddInt32(&ok, 1)
		}(u)
	}

	wg.Wait()

	log.Verbosef("dump %s complete, %d successfully dumped", id, ok)
}
