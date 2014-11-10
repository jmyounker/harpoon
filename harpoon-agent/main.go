package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"time"
)

var (
	// Version is a state variable, written at the link stage. See Makefile.
	Version string

	// CommitID is a state variable, written at the link stage. See Makefile.
	CommitID string

	// ExternalReleaseVersion is a state variable, written at the link stage.
	// See Makefile.
	ExternalReleaseVersion string
)

var (
	heartbeatInterval = 3 * time.Second
	configuredVolumes = volumes{} // TODO: de-globalize
	agentCPU          float64     // TODO: de-globalize
	agentMem          int64       // TODO: de-globalize
	debug             bool        // TODO: de-globalize
	logAddr           string      // TODO: de-globalize
)

func main() {
	var (
		showVersion   = flag.Bool("version", false, "print version")
		containerRoot = flag.String("run", "/run/harpoon", "filesytem root for packages")
		addr          = flag.String("addr", ":3333", "address to listen on")
		portsStart    = flag.Uint64("ports.start", 30000, "starting of port allocation range")
		portsEnd      = flag.Uint64("ports.end", 32767, "ending of port allocation range")
	)
	flag.Var(&configuredVolumes, "vol", "repeatable list of available volumes")
	flag.Float64Var(&agentCPU, "cpu", systemCPU(), "CPU resources to make available")
	flag.Int64Var(&agentMem, "mem", systemMem(), "memory (MB) resources to make available")
	flag.BoolVar(&debug, "debug", false, "debug logging")
	flag.StringVar(&logAddr, "log.addr", ":3334", "address for log communications")

	flag.Parse()

	if *showVersion {
		fmt.Printf("version %s (%s) %s\n", Version, CommitID, ExternalReleaseVersion)
		os.Exit(0)
	}

	if *portsStart > math.MaxUint16 {
		log.Fatalf("port range start must be between 0 and %d", math.MaxUint16)
	}
	portsStart16 := uint16(*portsStart)

	if *portsEnd > math.MaxUint16 {
		log.Fatalf("port range end must be between 0 and %d", math.MaxUint16)
	}
	portsEnd16 := uint16(*portsEnd)

	if portsStart16 >= portsEnd16 {
		log.Fatal("port range start must be before port range end")
	}

	r := newRegistry()
	pdb := newPortDB(portsStart16, portsEnd16)
	defer pdb.exit()
	api := newAPI(*containerRoot, r, pdb)

	go receiveLogs(r)

	http.Handle("/", api)

	go func() {
		recoverContainers(*containerRoot, r, pdb)

		r.acceptStateUpdates()

		if r.len() > 0 {
			time.Sleep(3 * heartbeatInterval) // wait for runners to check in
		}

		api.enable()
	}()

	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

type volumes map[string]struct{}

func (*volumes) String() string           { return "" }
func (v *volumes) Set(value string) error { (*v)[value] = struct{}{}; return nil }
