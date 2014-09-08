package main

import (
	"log"
	"os"
	"syscall"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type signalHandler struct {
	sigc chan os.Signal
	s    Supervisor
}

func newSignalHandler(sigc chan os.Signal, s Supervisor) *signalHandler {
	return &signalHandler{sigc, s}
}

func (h *signalHandler) Run() {
	var (
		exited = h.s.Exited()
		statec = make(chan agent.ContainerProcessState, 1)
	)

	select {
	case <-exited:
		return
	case <-h.sigc:
	}

	h.s.Stop(syscall.SIGTERM)

	h.s.Subscribe(statec)
	defer h.s.Unsubscribe(statec)

	for {
		select {
		case <-exited:
			return

		case <-h.sigc:
			h.s.Stop(syscall.SIGKILL)

		case state := <-statec:
			if state.Up || state.Restarting {
				continue
			}

			if err := h.s.Exit(); err != nil {
				log.Println("unable to exit supervisor: ", err)
			}
		}
	}
}
