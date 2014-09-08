package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ln, err := net.Listen("unix", "./control")
	if err != nil {
		log.Fatal("unable to listen on ./control: ", err)
	}
	defer ln.Close()

	var sigc = make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM, syscall.SIGINT)

	var (
		container     = newContainer("./container.json", "./rootfs", os.Args[1:])
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
