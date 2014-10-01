package main

import (
	"flag"
	"log"
	"math"
	"net/http"
	"os"
	"time"
)

var (
	heartbeatInterval = 3 * time.Second

	addr              = flag.String("addr", ":3333", "address to listen on")
	configuredVolumes = volumes{}

	agentTotalMem int64
	agentTotalCPU int64

	hostname string

	logAddr = flag.String("log.addr", ":3334", "address for log communications")
	debug   = flag.Bool("debug", false, "log verbosely for debugging, and only for debugging")
)

func init() {
	name, err := os.Hostname()
	if err != nil {
		log.Fatal("unable to get hostname: ", err)
	}
	hostname = name
}

func main() {
	flag.Int64Var(&agentTotalCPU, "cpu", -1, "available cpu resources (-1 to use all cpus)")
	flag.Int64Var(&agentTotalMem, "mem", -1, "available memory resources in MB (-1 to use all)")
	flag.Var(&configuredVolumes, "v", "repeatable list of available volumes")
	containerRoot := flag.String("run", "/run/harpoon", "filesytem root for packages")
	portRangeStart64 := flag.Uint64("ports.start", 30000, "starting of port allocation range")
	portRangeEnd64 := flag.Uint64("ports.end", 32767, "ending of port allocation range")

	flag.Parse()

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

	if agentTotalCPU == -1 {
		agentTotalCPU = systemCPUs()
	}

	if agentTotalMem == -1 {
		mem, err := systemMemoryMB()
		if err != nil {
			log.Fatal("unable to get available memory: ", err)
		}

		agentTotalMem = mem
	}

	var (
		r   = newRegistry()
		pr  = newPortRange(portRangeStart, portRangeEnd)
		api = newAPI(*containerRoot, r, pr)
	)

	go receiveLogs(r)

	http.Handle("/", api)

	go func() {
		// recover our state from disk
		recoverContainers(*containerRoot, r)

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
