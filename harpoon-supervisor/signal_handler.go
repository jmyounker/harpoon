package main

import (
	"log"
	"os"
	"syscall"
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
		statec = make(chan ContainerProcessState, 1)
	)

	select {
	case <-exited:
		return
	case <-h.sigc:
	}

	h.s.Notify(statec)
	defer h.s.Unnotify(statec)

	h.s.Stop(syscall.SIGTERM)

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
