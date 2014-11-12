package agent

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoonctl/neu/log"
)

var createCommand = cli.Command{
	Name:        "create",
	Usage:       "create <config.json> <id>",
	Description: "Creates (allocates) and starts a container.",
	Action:      createAction,
	Flags:       []cli.Flag{},
}

func createAction(c *cli.Context) {
	if len(c.Args()) != 2 {
		log.Fatalf("usage: create <config.json> <id>")
	}

	var (
		filename = c.Args()[0]
		id       = c.Args()[1]
	)

	f, err := os.Open(filename)
	if err != nil {
		log.Fatalf("%s: %s", filename, err)
	}

	var cfg agent.ContainerConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		log.Fatalf("%s: %s", filename, err)
	}

	if err := cfg.Valid(); err != nil {
		log.Fatalf("%s: %s", filename, err)
	}

	u := chooseEndpoint(c)

	client, err := agent.NewClient(u.String())
	if err != nil {
		log.Fatalf("%s: %s", u.Host, err)
	}

	if err := client.Put(id, cfg); err != nil {
		log.Fatalf("%s: %s", u.Host, err)
	}

	log.Printf("%s: create %s (%s) OK", u.Host, id, filename)
}

func chooseEndpoint(c *cli.Context) *url.URL {
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

	fmt.Fprintf(w, " \tAGENT\tCPU\tTOTAL\tMEM\tTOTAL\n")
	for i, ur := range a {
		fmt.Fprintf(
			w,
			"%d\t%s\t%.2f\t%.2f\t%d\t%d\n",
			i+1,
			ur.URL.Host,
			ur.HostResources.CPU.Reserved,
			ur.HostResources.CPU.Total,
			ur.HostResources.Mem.Reserved,
			ur.HostResources.Mem.Total,
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
