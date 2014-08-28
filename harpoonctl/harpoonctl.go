package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/codegangsta/cli"
	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type harpoonctl struct {
	cluster cluster

	*tabwriter.Writer
}

func (c *harpoonctl) setAgents(ctx *cli.Context) error {
	var (
		cluster = ctx.GlobalString("cluster")
		agents  = ctx.GlobalStringSlice("agent")
	)

	if len(agents) > 0 && cluster != "" {
		return fmt.Errorf("cannot specify both agent and cluster flags")
	}

	if len(agents) == 0 && cluster == "" {
		if _, err := os.Stat("~/.harpoonctl/clusters/default"); err == nil {
			cluster = "default"
		} else {
			agents = []string{"localhost:3333"}
		}
	}

	if cluster != "" {
		a, err := c.loadCluster(fmt.Sprintf("~/.harpoonctl/clusters/%s", cluster))

		if err != nil {
			return fmt.Errorf("unable to load cluster: %s", err)
		}

		agents = a
	}

	for _, addr := range agents {
		client, err := agent.NewClient(fmt.Sprintf("http://%s", addr))
		if err != nil {
			return err
		}

		c.cluster = append(c.cluster, Agent{addr, client})
	}

	return nil
}

func (*harpoonctl) loadCluster(filename string) ([]string, error) {
	if filename[0] == '~' {
		filename = os.Getenv("HOME") + filename[1:]
	}

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	var (
		scanner = bufio.NewScanner(f)
		agents  = []string{}
	)

	for scanner.Scan() {
		agents = append(agents, scanner.Text())
	}

	return agents, scanner.Err()
}

func (c *harpoonctl) resources(ctx *cli.Context) {
	resources, err := c.cluster.Resources()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Fprintln(c, "AGENT	MEM	RESERVED	CPU	RESERVED	VOLUMES")

	for a, r := range resources {
		fmt.Fprintf(
			c,
			"%s	%d	%d	%f	%f	%s\n",
			a,
			int(r.Memory.Total),
			int(r.Memory.Reserved),
			r.CPUs.Total,
			r.CPUs.Reserved,
			"-", // TODO: list volumes
		)
	}

	c.Flush()
}

func (c *harpoonctl) ps(ctx *cli.Context) {
	containers, err := c.cluster.Containers()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Fprintln(
		c,
		"AGENT	ID	COMMAND	STATUS",
	)

	for a, cs := range containers {
		for id, container := range cs {
			fmt.Fprintf(
				c,
				"%s	%s	%s	%s\n",
				a,
				id,
				container.ContainerConfig.Command.Exec[0],
				container.ContainerStatus,
			)
		}
	}

	c.Flush()
}

func (c *harpoonctl) status(ctx *cli.Context) {
	args := ctx.Args()
	if len(args) < 1 {
		log.Fatal("no container id provided")
	}
	id := args[0]

	container, err := c.cluster.Get(id)
	if err != nil {
		log.Fatal(err)
	}

	buf, err := json.MarshalIndent(container, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Fprintf(os.Stdout, "%s\n", buf)
}

func (c *harpoonctl) run(ctx *cli.Context) {
	args := ctx.Args()
	if len(args) < 1 {
		log.Fatal("usage: harpoonctl run <config.json> [id]")
	}
	filename := args[0]

	var id = uuid()
	if len(args) == 2 {
		id = args[1]
	}

	configFile, err := os.Open(filename)
	if err != nil {
		log.Fatal("unable to open config file: ", err)
	}
	defer configFile.Close()

	var config agent.ContainerConfig
	if err := json.NewDecoder(configFile).Decode(&config); err != nil {
		log.Fatal("unable to parse config file: ", err)
	}

	resources, err := c.cluster.Resources()
	if err != nil {
		log.Fatal("unable to get resources: ", err)
	}

	options := map[int]string{} // map of option to agent

	i := 0
	fmt.Fprintln(c, "	AGENT	MEM	RESERVED	CPU	RESERVED	VOLUMES")

	for a, r := range resources {
		i++
		options[i] = a

		fmt.Fprintf(
			c,
			"%d)	%s	%d	%d	%f	%f	%s\n",
			i,
			a,
			int(r.Memory.Total),
			int(r.Memory.Reserved),
			r.CPUs.Total,
			r.CPUs.Reserved,
			"-", // TODO: list volumes
		)
	}

	c.Flush()
	fmt.Fprintf(os.Stdout, "\nSelect an agent [default: 1]: ")

	var choice = 1
	if n, err := fmt.Fscanf(os.Stdin, "%d", &choice); n > 0 && err != nil {
		log.Fatal("unable to read choice: ", err)
	}

	for _, a := range c.cluster {
		if a.Addr == options[choice] {
			if err := a.Put(id, config); err != nil {
				log.Fatal("unable to start: ", err)
			}

			c.wait(a, id)
			return
		}
	}
}

func (*harpoonctl) wait(a Agent, id string) {
	events, stopper, err := a.Events()
	if err != nil {
		log.Fatal("unable to get event stream: ", err)
	}
	defer stopper.Stop()

	for containers := range events {
		container, ok := containers[id]
		if !ok {
			continue
		}

		switch status := container.ContainerStatus; status {
		case agent.ContainerStatusRunning, agent.ContainerStatusFailed, agent.ContainerStatusFinished:
			log.Println(status)
			return
		}
	}
}

func (c *harpoonctl) start(ctx *cli.Context) {
	args := ctx.Args()
	if len(args) != 1 {
		log.Fatal("usage: harpoonctl start <id>")
	}
	id := args[0]

	if err := c.cluster.Start(id); err != nil {
		log.Fatal("unable to start container: ", err)
	}
}

func (c *harpoonctl) stop(ctx *cli.Context) {
	args := ctx.Args()
	if len(args) != 1 {
		log.Fatal("usage: harpoonctl stop <id>")
	}
	id := args[0]

	if err := c.cluster.Stop(id); err != nil {
		log.Fatal("unable to stop container: ", err)
	}
}

func (c *harpoonctl) destroy(ctx *cli.Context) {
	args := ctx.Args()
	if len(args) != 1 {
		log.Fatal("usage: harpoonctl destroy <id>")
	}
	id := args[0]

	if err := c.cluster.Delete(id); err != nil {
		log.Fatal("unable to destroy container: ", err)
	}
}

func (c *harpoonctl) logs(ctx *cli.Context) {
	args := ctx.Args()
	if len(args) == 0 {
		log.Fatal("usage: harpoonctl logs <id>")
	}
	id := args[0]

	lines, stopper, err := c.cluster.Log(id, 0)
	if err != nil {
		log.Fatal("unable to get logs: ", err)
	}
	defer stopper.Stop()

	for line := range lines {
		log.Printf("%s", line)
	}
}
