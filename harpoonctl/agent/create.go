package agent

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var createCommand = cli.Command{
	Name:        "create",
	Usage:       "create <config.json> <id>",
	Description: "Creates (allocates) a container.",
	Action:      createAction,
	Flags:       []cli.Flag{timeoutFlag},
}

var timeoutFlag = cli.DurationFlag{
	Name:  "t, timeout",
	Value: agent.DefaultDownloadTimeout + (30 * time.Second), // +30s for call overhead
	Usage: "Total timeout for remote artifact download and invocation",
}

func createAction(c *cli.Context) {
	if len(c.Args()) != 2 {
		log.Fatalf("usage: create <config.json> <id>")
	}

	var (
		filename = c.Args()[0]
		id       = c.Args()[1]
		timeout  = c.Duration("timeout")
	)

	create(filename, id, timeout, false)
}

func create(filename, id string, timeout time.Duration, andStart bool) {
	f, err := os.Open(filename)
	if err != nil {
		log.Fatalf("%s: %s", filename, err)
	}
	defer f.Close()

	var cfg agent.ContainerConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		log.Fatalf("%s: %s", filename, err)
	}

	if err := cfg.Valid(); err != nil {
		log.Fatalf("%s: %s", filename, err)
	}

	u := chooseEndpoint()

	client, err := agent.NewClient(u.String())
	if err != nil {
		log.Fatalf("%s: %s", u.Host, err)
	}

	// Start listening for client creation before we issue the Create call,
	// otherwise there is a small window in which we can lose responses from
	// the client.
	wanted := map[agent.ContainerStatus]struct{}{
		agent.ContainerStatusCreated: struct{}{},
		agent.ContainerStatusDeleted: struct{}{},
	}
	wc := client.Wait(id, wanted, timeout)

	// Issue create.
	if err := client.Put(id, cfg); err != nil {
		log.Fatalf("%s: %s", u.Host, err)
	}

	// Check results from create.
	w := <-wc
	if w.Err != nil {
		log.Fatalf("%s: %s", u.Host, w.Err)
	}

	if w.Status == agent.ContainerStatusDeleted {
		log.Fatalf("%s: container creation failed", id)
	}

	if andStart {
		if err := client.Start(id); err != nil {
			log.Fatalf("%s: %s", u.Host, err)
		}
		log.Printf("%s: run %s (%s) OK", u.Host, id, filename)
	} else {
		log.Printf("%s: create %s (%s) OK", u.Host, id, filename)
	}
}

func chooseEndpoint() *url.URL {
	if len(endpoints) <= 0 {
		panic("impossible")
	}

	if len(endpoints) == 1 {
		return endpoints[0]
	}

	var (
		w  = tabwriter.NewWriter(os.Stderr, 0, 2, 2, ' ', 0)
		ch = make(chan urlResources, len(endpoints))
	)

	for _, u := range endpoints {
		go func(u *url.URL) {
			var r agent.HostResources
			defer func() { ch <- urlResources{u, r} }()

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

	a := make([]urlResources, cap(ch))
	for i := 0; i < cap(ch); i++ {
		a[i] = <-ch
	}

	sort.Sort(byURL(a))

	fmt.Fprintf(w, " \tAGENT\tCPU FREE\tCPU TOTAL\tMEM FREE\tMEM TOTAL\tVOLUMES\n")
	for i, ur := range a {
		fmt.Fprintf(
			w,
			"%d\t%s\t%.2f\t%.2f\t%d\t%d\t%s\n",
			i+1,
			ur.URL.Host,
			ur.HostResources.CPU.Total-ur.HostResources.CPU.Reserved,
			ur.HostResources.CPU.Total,
			ur.HostResources.Mem.Total-ur.HostResources.Mem.Reserved,
			ur.HostResources.Mem.Total,
			strings.Join(ur.HostResources.Volumes, ", "),
		)
	}

	w.Flush()

	fmt.Fprintf(os.Stderr, "\nSelect an agent [1]: ")

	var choice = 1
	if n, err := fmt.Fscanf(os.Stdin, "%d", &choice); n > 0 && err != nil {
		log.Fatalf("unable to read choice (%s)", err)
	}

	if choice <= 0 || choice > len(a) {
		log.Fatalf("invalid choice %d", choice)
	}

	return a[choice-1].URL
}

type urlResources struct {
	*url.URL
	agent.HostResources
}

type byURL []urlResources

func (a byURL) Len() int           { return len(a) }
func (a byURL) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byURL) Less(i, j int) bool { return a[i].URL.String() < a[j].URL.String() }
