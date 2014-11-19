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
	Usage:       "Stream events from agent(s)",
	Description: "Displays events streaming from agent(s).",
	Action:      eventsAction,
}

func eventsAction(c *cli.Context) {
	var (
		epec = make(chan (<-chan agent.StateEvent), len(endpoints)) // endpoint event channels
		wg   = sync.WaitGroup{}
	)

	for _, u := range endpoints {
		go func(u *url.URL) {
			var c <-chan agent.StateEvent
			defer func() { epec <- c }()

			client, err := agent.NewClient(u.String())
			if err != nil {
				log.Warnf("%s: %s", u.Host, err)
				return
			}

			log.Verbosef("%s: connected to host", u.Host)

			c, _, err = client.Events()
			if err != nil {
				log.Warnf("%s: %s", u.Host, err)
				return
			}
			log.Verbosef("%s: connected to event stream", u.Host)
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
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			for e := range ec {
				m, err := json.Marshal(e)
				if err != nil {
					log.Printf("error: unparsable event: %s", err)
					continue
				}
				fmt.Fprintf(os.Stdout, fmt.Sprintf("%s\n", string(m)))
			}
		}(&wg)
	}

	wg.Wait()
}
