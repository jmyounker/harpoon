package scheduler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	schedulerapi "github.com/soundcloud/harpoon/harpoon-scheduler/api"
	"github.com/soundcloud/harpoon/harpoonctl/log"
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
	var (
		l = c.Bool("long")
		w = tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	)

	m, err := currentState()
	if err != nil {
		log.Fatalf("%s: %s", endpoint.Host, err)
	}

	a := []string{}

	if l {
		fmt.Fprintf(w, "AGENT\tID\tSTATUS\tCMD\tARTIFACT\n")
		for endpoint, se := range m {
			for _, ci := range se.Containers {
				a = append(a, fmt.Sprintf(
					"%s\t%s\t%s\t%s\t%s\n",
					endpoint,
					ci.ID,
					ci.ContainerStatus,
					ci.Command.Exec[0],
					ci.ArtifactURL,
				))
			}
		}
	} else {
		fmt.Fprintf(w, "AGENT\tID\tSTATUS\n")
		for endpoint, se := range m {
			for _, ci := range se.Containers {
				a = append(a, fmt.Sprintf(
					"%s\t%s\t%s\n",
					endpoint,
					ci.ID,
					ci.ContainerStatus,
				))
			}
		}
	}

	// Don't display header if we didn't have any rows.
	if len(a) <= 0 {
		log.Verbosef("no tasks")
		return
	}

	sort.Sort(sort.StringSlice(a))

	for _, s := range a {
		fmt.Fprintf(w, s)
	}

	w.Flush()
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