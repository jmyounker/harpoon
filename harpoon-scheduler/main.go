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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/soundcloud/harpoon/harpoon-scheduler/agentrepr"
	"github.com/soundcloud/harpoon/harpoon-scheduler/api"
	"github.com/soundcloud/harpoon/harpoon-scheduler/registry"
	"github.com/soundcloud/harpoon/harpoon-scheduler/reprproxy"
	"github.com/soundcloud/harpoon/harpoon-scheduler/xf"
)

var (
	// Override at link stage (see Makefile)
	Version                string
	CommitID               string
	ExternalReleaseVersion string
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Lmicroseconds | log.Lshortfile)

	var (
		debug   = flag.Bool("debug", false, "enable debug logging")
		listen  = flag.String("listen", ":4444", "HTTP listen address")
		version = flag.Bool("version", false, "print version")
		persist = flag.String("persist", "scheduler-registry.json", "filename to persist registry state")
		agents  = multiagent{}
	)
	flag.Var(&agents, "agent", "repeatable list of agent endpoints")
	flag.Parse()

	if *version {
		fmt.Printf("version %s (%s) %s\n", Version, CommitID, ExternalReleaseVersion)
		os.Exit(0)
	}

	if *debug {
		agentrepr.Debugf = log.Printf
		xf.Debugf = log.Printf
		reprproxy.Debugf = log.Printf
	}

	log.Printf("%d agent(s)", len(agents.slice()))

	var (
		r = registry.New(*persist)
		d = reprproxy.StaticAgentDiscovery(agents.slice())
		p = reprproxy.New(d)
		w = logWriter{}
	)

	go xf.Transform(r, p, p)

	http.Handle("/metrics", api.Log(w, prometheus.Handler()))
	http.Handle("/api/v0/", api.Log(w, api.NewHandler(p, r)))
	http.Handle("/favicon.ico", http.NotFoundHandler())
	http.Handle("/", http.RedirectHandler("/api/v0/snapshot/", http.StatusTemporaryRedirect))

	errc := make(chan error, 2)

	go func() {
		log.Printf("listening on %s", *listen)
		errc <- http.ListenAndServe(*listen, nil)
	}()

	go func() {
		errc <- fmt.Errorf("%s", <-interrupt())
	}()

	log.Fatal(<-errc)
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

type logWriter struct{}

func (w logWriter) Write(b []byte) (int, error) {
	log.Print(string(b))
	return len(b), nil
}

func interrupt() chan os.Signal {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT)
	return c
}
