package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	var (
		httpAddress      = flag.String("http.address", ":4444", "HTTP listen address")
		registryFilename = flag.String("registry.filename", "scheduler-registry.json", "persistence filename")
		agents           = multiagent{}
	)
	flag.Var(&agents, "agent", "repeatable list of agent endpoints")
	flag.Parse()

	var (
		discovery   = staticAgentDiscovery(agents.slice())
		shepherd    = newRealShepherd(discovery)
		registry    = newRealRegistry(*registryFilename)
		transformer = newTransformer(shepherd, registry, shepherd)
		scheduler   = newRealScheduler(shepherd, registry)
		api         = newAPI(scheduler, registry, shepherd)
	)

	defer shepherd.quit()
	defer registry.quit()
	defer transformer.quit()
	defer scheduler.quit()

	log.Printf("there are %d static agent(s)", len(agents.slice()))
	log.Printf("the shepherd flock size is %d", shepherd.size())

	err := make(chan error, 2)

	go func() {
		log.Printf("listening on %s", *httpAddress)
		err <- http.ListenAndServe(*httpAddress, api)
	}()

	go func() {
		err <- fmt.Errorf("%s", <-interrupt())
	}()

	log.Print(<-err)
}

type multiagent map[string]struct{}

func (*multiagent) String() string { return "" }

func (a *multiagent) Set(value string) error {
	if !strings.HasPrefix(strings.ToLower(value), "http") {
		value = "http://" + value
	}

	if _, err := url.Parse(value); err != nil {
		return fmt.Errorf("invalid agent endpoint: %s", err)
	}

	(*a)[value] = struct{}{}

	log.Printf("agent: %s", value)

	return nil
}

func (a multiagent) slice() []string {
	s := make([]string, 0, len(a))

	for value := range a {
		s = append(s, value)
	}

	return s
}

func interrupt() <-chan os.Signal {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT)
	return c
}
