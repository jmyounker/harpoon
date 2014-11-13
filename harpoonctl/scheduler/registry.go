package scheduler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
	schedulerapi "github.com/soundcloud/harpoon/harpoon-scheduler/api"
	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var registryCommand = cli.Command{
	Name:        "registry",
	Usage:       "Print scheduler registry",
	Description: "Display current view of the scheduler's persisted registry, containing all scheduled jobs.",
	Action:      registryAction,
	Flags:       []cli.Flag{},
}

func registryAction(c *cli.Context) {
	var (
		m = map[string]configstore.JobConfig{}
		w = tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	)

	resp, err := http.Get(endpoint.String() + schedulerapi.APIVersionPrefix + schedulerapi.APIRegistryPath)
	if err != nil {
		log.Fatalf("%s", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		log.Fatalf("%s: when parsing response: %s", endpoint.Host, err)
	}

	if len(m) <= 0 {
		log.Verbosef("no jobs in the registry")
		return
	}

	fmt.Fprint(w, "HASH\tJOB\tENV\tPROD\tSCALE\tCMD\tARTIFACT\n")
	for _, c := range m {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			c.Hash(),
			c.Job,
			c.Environment,
			c.Product,
			c.Scale,
			c.Command.Exec[0],
			c.ArtifactURL,
		)
	}
	w.Flush()
}
