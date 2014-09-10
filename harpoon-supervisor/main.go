package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	controlFileName   = "./control"
	containerFileName = "./container.json"
	rootfsFileName    = "./rootfs"

	containerInitName = "harpoon-container-init"
)

func main() {
	ln, err := net.Listen("unix", controlFileName)
	if err != nil {
		log.Fatal("unable to listen on %q: ", controlFileName, err)
	}
	defer ln.Close()

	var sigc = make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM, syscall.SIGINT)

	var (
		container     = newContainer(containerFileName, rootfsFileName, os.Args[1:])
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
