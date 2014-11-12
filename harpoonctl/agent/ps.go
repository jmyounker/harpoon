package agent

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var psCommand = cli.Command{
	Name:        "ps",
	Usage:       "Print instances on agent(s)",
	Description: "Display instances running on agent(s).",
	Action:      psAction,
	Flags:       []cli.Flag{longFlag},
}

func psAction(c *cli.Context) {
	var (
		w  = tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		l  = c.Bool("long")
		f  = func(host string, m map[string]agent.ContainerInstance) []string { return []string{} }
		ch = make(chan []string, len(endpoints))
	)

	if l {
		fmt.Fprint(w, "AGENT\tID\tSTATUS\tCPUTIME\tMEM USED\tFDS\tRESTARTS\tOOMS\tCMD\tPORTS\tRC\n")
		f = func(host string, m map[string]agent.ContainerInstance) []string {
			a := make([]string, 0, len(m))
			for id, ci := range m {
				a = append(a, fmt.Sprintf(
					"%s\t%s\t%s\t%d\t%dM\t%d\t%d\t%d\t%s\t%+v\t%d\n",
					host,
					id,
					ci.ContainerStatus,
					ci.ContainerMetrics.CPUTime,
					ci.ContainerMetrics.MemoryUsage/1024/1024,
					ci.FD,
					ci.Restarts,
					ci.OOMs,
					ci.Command.Exec[0],
					ci.Ports,
					ci.ExitStatus,
				))
			}
			return a
		}
	} else {
		fmt.Fprint(w, "AGENT\tID\tSTATUS\tCMD\n")
		f = func(host string, m map[string]agent.ContainerInstance) []string {
			a := make([]string, 0, len(m))
			for id, ci := range m {
				a = append(a, fmt.Sprintf(
					"%s\t%s\t%s\t%s\n",
					host,
					id,
					ci.ContainerStatus,
					ci.Command.Exec[0],
				))
			}
			return a
		}
	}

	for _, u := range endpoints {
		go func(u *url.URL) {
			var m map[string]agent.ContainerInstance
			defer func() { ch <- f(u.Host, m) }()

			c, err := agent.NewClient(u.String())
			if err != nil {
				log.Warnf("%s: %s", u.Host, err)
				return
			}

			m, err = c.Containers()
			if err != nil {
				log.Warnf("%s: %s", u.Host, err)
				return
			}
		}(u)
	}

	first := make([][]string, cap(ch))
	for i := 0; i < cap(ch); i++ {
		first[i] = <-ch
	}

	a := []string{}
	for _, slice := range first {
		for _, s := range slice {
			a = append(a, s)
		}
	}

	sort.StringSlice(a).Sort()

	for _, s := range a {
		fmt.Fprintf(w, s)
	}

	// Don't display header if we didn't have any rows.
	if len(a) > 0 {
		w.Flush()
	}
}
