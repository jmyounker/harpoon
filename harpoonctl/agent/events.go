package agent

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sync"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var eventsCommand = cli.Command{
	Name:        "events",
	Usage:       "events",
	Description: "Streams events from an agent.",
	Action:      eventsAction,
	Flags:       []cli.Flag{},
}

func eventsAction(c *cli.Context) {
	type cStopper struct {
		c <-chan string
		agent.Stopper
	}

	var (
		id   = c.Args().First()
		epec = make(chan (<-chan agent.StateEvent), len(endpoints)) // endpoint event channels
		wg   = sync.WaitGroup{}
	)

	// I should probaly shut this thing down gracefully, but it works.
	for _, u := range endpoints {
		go func(u *url.URL) {
			var c <-chan agent.StateEvent
			defer func() { epec <- c }()

			client, err := agent.NewClient(u.String())
			if err != nil {
				log.Warnf("%s: %s", u.Host, err)
				return
			}

			log.Verbosef("%s: checking %s...", u.Host, id)

			c, _, err = client.Events()
			if err != nil {
				log.Warnf("%s: %s", u.Host, err)
				return
			}

			log.Verbosef("%s: %s found", u.Host, id)
		}(u)
	}

	// Loop over all the event channels from the endpoints we were
	// able to successfully connect to.
	for i := 0; i < cap(epec); i++ {
		// Event channel from set of endpoint event channels
		ec := <-epec
		if ec == nil {
			continue
		}

		wg.Add(1)

		// Spew events from this one agent's channel
		go func() {
			for e := range ec {
				m, err := json.Marshal(e)
				if err != nil {
					log.Printf("error: unparsable event: %s", err)
					continue
				}
				fmt.Fprintf(os.Stdout, fmt.Sprintf("%s\n", string(m)))
			}
		}()
	}

	wg.Wait()
}
