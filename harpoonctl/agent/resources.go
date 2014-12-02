package agent

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var resourcesCommand = cli.Command{
	Name:        "resources",
	Usage:       "Print agent host resources",
	Description: "Display instantaneous total and reserved CPU, memory, etc. resources of agent(s).",
	Action:      resourcesAction,
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "l, long",
			Usage: "long output",
		},
	},
}

func resourcesAction(c *cli.Context) {
	var (
		w  = tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		l  = c.Bool("long")
		f  = func(host string, r agent.HostResources) string { return "" }
		ch = make(chan string, len(endpoints))
	)

	if l {
		fmt.Fprint(w, "AGENT\tCPU\tTOTAL\tMEM\tTOTAL\tSTORAGE\tTOTAL\tVOLUMES\n")
		f = func(host string, r agent.HostResources) string {
			return fmt.Sprintf(
				"%s\t%.2f\t%.2f\t%d\t%d\t%d\t%d\t%s\n",
				host,
				r.CPU.Reserved,
				r.CPU.Total,
				r.Mem.Reserved,
				r.Mem.Total,
				r.Storage.Reserved,
				r.Storage.Total,
				strings.Join(r.Volumes, ", "),
			)
		}
	} else {
		fmt.Fprint(w, "AGENT\tCPU\tTOTAL\tMEM\tTOTAL\tVOLUMES\n")
		f = func(host string, r agent.HostResources) string {
			return fmt.Sprintf(
				"%s\t%.2f\t%.2f\t%d\t%d\t%s\n",
				host,
				r.CPU.Reserved,
				r.CPU.Total,
				r.Mem.Reserved,
				r.Mem.Total,
				strings.Join(r.Volumes, ", "),
			)
		}
	}

	for _, u := range endpoints {
		go func(u *url.URL) {
			var r agent.HostResources
			defer func() { ch <- f(u.Host, r) }()

			c, err := agent.NewClient(u.String())
			if err != nil {
				log.Warnf("%s: %s", u.Host, err)
				return
			}

			r, err = c.Resources()
			if err != nil {
				log.Warnf("%s: %s", u.Host, err)
				return
			}
		}(u)
	}

	a := make([]string, cap(ch))
	for i := 0; i < cap(ch); i++ {
		a[i] = <-ch
	}

	// Don't display header if we didn't have any rows.
	if len(a) <= 0 {
		log.Verbosef("no agents")
		return
	}

	sort.StringSlice(a).Sort()

	for _, s := range a {
		fmt.Fprintf(w, s)
	}

	w.Flush()
}
