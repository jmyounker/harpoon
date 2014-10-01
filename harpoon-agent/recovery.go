package main

import (
	"io"
	"io/ioutil"
	"log"
	"net"
	"path/filepath"
	"time"
)

// recoverContainers restores container states from disk, e.g., after
// harpoon-agent is restarted.
//
// BUG: all components are not yet available to support recovery, so this
// function will instead kill all running containers.
func recoverContainers(containerRoot string, r *registry) {
	matches, err := filepath.Glob(filepath.Join(containerRoot, "*", "control"))
	if err != nil {
		log.Println("unable to scan rundir for containers: ", err)
		return
	}

	log.Printf("%d containers may still be running; killing now", len(matches))

	killContainer := func(conn net.Conn) {
		done := make(chan struct{})

		// ignore state updates
		go func() { io.Copy(ioutil.Discard, conn); close(done) }()

		for {
			conn.Write([]byte("event: kill\n\nevent: exit\n\n"))

			select {
			case <-done:
				return

			case <-time.After(10 * time.Millisecond):
			}
		}
	}

	for _, controlPath := range matches {
		id := filepath.Base(filepath.Dir(controlPath))

		conn, err := net.Dial("unix", controlPath)
		if err != nil {
			continue
		}

		log.Printf("container[%s]: killing", id)
		killContainer(conn)
		log.Printf("container[%s]: killed", id)
	}
}
