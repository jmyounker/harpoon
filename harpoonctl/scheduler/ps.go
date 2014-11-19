package scheduler

import (
	"net/url"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	schedulerapi "github.com/soundcloud/harpoon/harpoon-scheduler/api"
	"github.com/soundcloud/harpoon/harpoonctl/log"
	agentcmd "github.com/soundcloud/harpoon/harpoonctl/agent"
)

var psCommand = cli.Command{
	Name:        "ps",
	Usage:       "Print tasks",
	Description: "Display all tasks (containers) that the scheduler is aware of.",
	Action:      psAction,
	Flags:       []cli.Flag{longFlag},
}

var longFlag = cli.BoolFlag{
	Name:  "l, long",
	Usage: "long output",
}

func psAction(c *cli.Context) {
	m, err := currentState()
	if err != nil {
		log.Fatalf("%s: %s", endpoint.Host, err)
	}

	agentcmd.WriteContainerPS(
		tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0),
		se2ci(m),
		c.Bool("long"),
	)
}

func currentState() (map[string]agent.StateEvent, error) {
	resp, err := http.Get(endpoint.String() + schedulerapi.APIVersionPrefix + schedulerapi.APIProxyPath)
	if err != nil {
		return map[string]agent.StateEvent{}, err
	}
	defer resp.Body.Close()

	var m map[string]agent.StateEvent
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return map[string]agent.StateEvent{}, fmt.Errorf("when parsing response: %s", err)
	}

	return m, nil
}

func se2ci(m map[string]agent.StateEvent) map[string]map[string]agent.ContainerInstance {
	out := map[string]map[string]agent.ContainerInstance{}

	for endpoint, se := range m {
		host := endpoint2host(endpoint)

		for id, ci := range se.Containers {
			if _, ok := out[host]; !ok {
				out[host] = map[string]agent.ContainerInstance{}
			}

			out[host][id] = ci
		}
	}

	return out
}

func endpoint2host(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		log.Warnf("parsing endpoint %s: %s", err)
		return endpoint
	}

	return u.Host
}