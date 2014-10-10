package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	controlFileName   = "./control"
	agentFileName     = "./agent.json"
	containerFileName = "./container.json"
	rootfsFileName    = "./rootfs"

	containerInitName = "harpoon-container-init"
)

func main() {
	var (
		hostname = flag.String("hostname", "", "hostname")
		id       = flag.String("id", "", "container ID")

		telemetryAddr   = flag.String("telemetry.address", "", "address for serving telemetry")
		telemetryLabels = telemetryLabels{}
	)

	flag.Var(&telemetryLabels, "telemetry.label", "repeatable list of telemetry labels (K=V)")
	flag.Parse()

	setupMetrics(prometheus.Labels(telemetryLabels))

	go func() {
		http.Handle("/metrics", prometheus.Handler())
		http.ListenAndServe(*telemetryAddr, nil)
	}()

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

	restartTimer := func() <-chan time.Time {
		return time.After(time.Second)
	}

	supervisor.Run(time.Tick(3*time.Second), restartTimer)
}
