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
	showVersion = flag.Bool("version", false, "print version")

	heartbeatInterval = 3 * time.Second

	addr              = flag.String("addr", ":3333", "address to listen on")
	configuredVolumes = volumes{}

	agentTotalMem uint64
	agentTotalCPU uint64

	hostname string

	logAddr = flag.String("log.addr", ":3334", "address for log communications")
	debug   = flag.Bool("debug", false, "log verbosely for debugging, and only for debugging")

	// Override at link stage (see Makefile)
	Version                string
	CommitID               string
	ExternalReleaseVersion string
)

func init() {
	name, err := os.Hostname()
	if err != nil {
		log.Fatal("unable to get hostname: ", err)
	}
	hostname = name
}

func main() {
	var cpu, mem int64

	flag.Int64Var(&cpu, "cpu", -1, "available cpu resources (-1 to use all cpus)")
	flag.Int64Var(&mem, "mem", -1, "available memory resources in MB (-1 to use all)")
	flag.Var(&configuredVolumes, "v", "repeatable list of available volumes")
	containerRoot := flag.String("run", "/run/harpoon", "filesytem root for packages")
	portRangeStart64 := flag.Uint64("ports.start", 30000, "starting of port allocation range")
	portRangeEnd64 := flag.Uint64("ports.end", 32767, "ending of port allocation range")

	flag.Parse()

	if *showVersion {
		fmt.Printf("version %s (%s) %s\n", Version, CommitID, ExternalReleaseVersion)
		os.Exit(0)
	}

	if *portRangeStart64 > math.MaxUint16 {
		log.Fatalf("port range start must be between 0 and %d", math.MaxUint16)
	}
	portRangeStart := uint16(*portRangeStart64)

	if *portRangeEnd64 > math.MaxUint16 {
		log.Fatalf("port range end must be between 0 and %d", math.MaxUint16)
	}
	portRangeEnd := uint16(*portRangeEnd64)

	if portRangeStart >= portRangeEnd {
		log.Fatal("port range start must be before port range end")
	}

	if cpu == -1 {
		agentTotalCPU = systemCPUs()
	} else {
		agentTotalCPU = uint64(cpu)
	}

	if mem == -1 {
		memory, err := systemMemoryMB()
		if err != nil {
			log.Fatal("unable to get available memory: ", err)
		}
		agentTotalMem = memory
	} else {
		agentTotalMem = uint64(mem)
	}

	r := newRegistry()
	pdb := newPortDB(portRangeStart, portRangeEnd)
	defer pdb.exit()
	api := newAPI(*containerRoot, r, pdb)

	go receiveLogs(r)

	http.Handle("/", api)

	go func() {
		// recover our state from disk
		recoverContainers(*containerRoot, r, pdb)

		// begin accepting runner updates
		r.acceptStateUpdates()

		if r.len() > 0 {
			// wait for runners to check in
			time.Sleep(3 * heartbeatInterval)
		}

		api.enable()
	}()

	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

type volumes map[string]struct{}

func (*volumes) String() string           { return "" }
func (v *volumes) Set(value string) error { (*v)[value] = struct{}{}; return nil }
