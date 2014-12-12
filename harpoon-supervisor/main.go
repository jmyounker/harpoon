package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	controlFileName   = "./control"
	agentFileName     = "./agent.json"
	containerFileName = "./container.json"
	rootfsFileName    = "./rootfs"

	containerInitName = "harpoon-container-init"
)

// Override at link stage (see Makefile)
var (
	Version                string
	CommitID               string
	ExternalReleaseVersion string
)

func main() {
	var (
		showVersion = flag.Bool("version", false, "print version")
		hostname    = flag.String("hostname", "", "hostname")
		id          = flag.String("id", "", "container ID")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("version %s (%s) %s\n", Version, CommitID, ExternalReleaseVersion)
		os.Exit(0)
	}

	if *hostname == "" {
		log.Fatal("hostname not supplied")
	}

	if *id == "" {
		log.Fatal("container ID not supplied")
	}

	ln, err := net.Listen("unix", controlFileName)
	if err != nil {
		log.Fatalf("unable to listen on %q: %s", controlFileName, err)
	}
	defer ln.Close()

	var sigc = make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM, syscall.SIGINT)

	var (
		container = newContainer(
			*hostname,
			*id,
			agentFileName,
			containerFileName,
			rootfsFileName,
			flag.Args(),
		)
		supervisor    = newSupervisor(container)
		signalHandler = newSignalHandler(sigc, supervisor)
		controller    = newController(ln, supervisor)
	)

	go signalHandler.Run()
	go controller.Run()

	supervisor.Run(time.Tick(3 * time.Second))
}
