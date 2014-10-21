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
	"time"
)

var (
	showVersion = flag.Bool("version", false, "print version")

	now   = time.Now
	after = time.After
	tick  = time.Tick

	// Override at link stage (see Makefile)
	Version                string
	CommitID               string
	ExternalReleaseVersion string
)

func main() {
	var (
		listen  = flag.String("listen", ":4444", "HTTP listen address")
		persist = flag.String("persist", "scheduler-registry.json", "filename to persist registry state")
		agents  = multiagent{}
		timeout = flag.Duration("timeout", time.Second, "timeout to set (un)scheduled job request as failed")
	)
	flag.Var(&agents, "agent", "repeatable list of agent endpoints")
	flag.Parse()

	if *showVersion {
		fmt.Printf("version %s (%s) %s\n", Version, CommitID, ExternalReleaseVersion)
		os.Exit(0)
	}

	var (
		discovery   = staticAgentDiscovery(agents.slice())
		shepherd    = newRealShepherd(discovery)
		registry    = newRealRegistry(*persist)
		transformer = newTransformer(shepherd, registry, shepherd, *timeout)
		api         = newAPI(registry, shepherd)
	)

	defer shepherd.quit()
	defer registry.quit()
	defer transformer.quit()

	log.Printf("the shepherd's state machine count is %d", shepherd.size())

	err := make(chan error, 2)

	go func() {
		log.Printf("listening on %s", *listen)
		err <- http.ListenAndServe(*listen, api)
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
