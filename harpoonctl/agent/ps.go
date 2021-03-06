package agent

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var psCommand = cli.Command{
	Name:        "ps",
	Usage:       "Display instances running on agent(s)",
	Description: psUsage,
	Action:      psAction,
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "l, long",
			Usage: "long output",
		},
	},
}

const psUsage = "ps"

func psAction(c *cli.Context) {
	var (
		w  = tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		ch = make(chan map[string]map[string]agent.ContainerInstance, len(endpoints)) // agent: container ID: instance
		m  = map[string]map[string]agent.ContainerInstance{}                          // merged
	)

	for _, u := range endpoints {
		go func(u *url.URL) {
			m := map[string]map[string]agent.ContainerInstance{}

			defer func() { ch <- m }()

			client, err := agent.NewClient(u.String())
			if err != nil {
				log.Warnf("%s: %s", u.Host, err)
				return
			}

			containers, err := client.Containers()
			if err != nil {
				log.Warnf("%s: %s", u.Host, err)
				return
			}

			m = map[string]map[string]agent.ContainerInstance{u.Host: containers}
		}(u)
	}

	for i := 0; i < cap(ch); i++ {
		for host, containers := range <-ch {
			if len(containers) <= 0 {
				continue
			}

			m[host] = containers
		}
	}

	WriteContainerPS(w, m, c.Bool("long"))
}

// WriteContainerPS writes a tab-delimited `ps` output for the containers to
// the passed writer.
func WriteContainerPS(w writeFlusher, m map[string]map[string]agent.ContainerInstance, long bool) {
	if len(m) <= 0 {
		return
	}

	lines := []string{}

	if long {
		fmt.Fprint(w, "AGENT\tID\tSTATUS\tCPU\tMEM\tFDS\t⟲\tOOMS\tCMD\tRC\tPORTS\n")
		for host, containers := range m {
			for id, ci := range containers {
				lines = append(lines, fmt.Sprintf(
					"%s\t%s\t%s\t%ds\t%dM\t%d\t%d\t%d\t%s\t%d\t%s\n",
					host,
					id,
					ci.ContainerStatus,
					ci.ContainerMetrics.CPUTime/1e9,           // ns -> s
					ci.ContainerMetrics.MemoryUsage/1024/1024, // B -> MB
					ci.FD,
					ci.Restarts,
					ci.OOMs,
					ci.Command.Exec[0],
					ci.ExitStatus,
					renderPorts(host, ci.Ports),
				))
			}
		}
	} else {
		fmt.Fprint(w, "AGENT\tID\tSTATUS\tCMD\n")
		for host, containers := range m {
			for id, ci := range containers {
				lines = append(lines, fmt.Sprintf(
					"%s\t%s\t%s\t%s\n",
					host,
					id,
					ci.ContainerStatus,
					ci.Command.Exec[0],
				))
			}
		}
	}

	sort.Sort(sort.StringSlice(lines))
	for _, line := range lines {
		fmt.Fprintf(w, line)
	}

	w.Flush()
}

func renderPorts(host string, ports map[string]uint16) string {
	var a []string
	for _, port := range ports {
		a = append(a, (&url.URL{
			Scheme: "http",
			Host:   strings.Split(host, ":")[0] + ":" + strconv.FormatUint(uint64(port), 10),
		}).String())
	}
	return strings.Join(a, ", ")
}

type writeFlusher interface {
	io.Writer
	Flush() error
}
