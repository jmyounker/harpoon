package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("harpoon-container: ")

	if os.Getpid() == 1 {
		if err := Init(); err != nil {
			log.Fatal("failed to initialize container:", err)
		}

		panic("unreachable")
	}

	var (
		heartbeatURL = os.Getenv("heartbeat_url")
		client       = newClient(heartbeatURL)
		c            = &Container{}
		heartbeat    = agent.Heartbeat{Status: "UP"}
	)

	f, err := os.Open("./container.json")
	if err != nil {
		heartbeat.Err = fmt.Sprintf("unable to open ./container.json: %s", err)
		shutDownContainer(client, c, &heartbeat)
		return
	}

	if err := json.NewDecoder(f).Decode(&c.container); err != nil {
		heartbeat.Err = fmt.Sprintf("unable to load ./container.json: %s", err)
		shutDownContainer(client, c, &heartbeat)
		return
	}

	monitorRunningContainer(client, c, &heartbeat)
	shutDownContainer(client, c, &heartbeat)
}

func monitorRunningContainer(client *client, c *Container, heartbeat *agent.Heartbeat) {
	var (
		transition  chan string
		transitionc = make(chan string, 1)
		desired     string
		statusc     = c.Start(transitionc)
	)

	for {
		select {
		case status, ok := <-statusc:
			if !ok {
				return
			}

			heartbeat.ContainerProcessStatus = status

			buf, _ := json.Marshal(status)
			log.Printf("container status: %s", buf)

			want, err := client.sendHeartbeat(*heartbeat)
			if err != nil {
				log.Println("unable to send heartbeat: ", err)
				continue
			}

			desired = want
			transition = transitionc

		case transition <- desired:
			transition = nil
		}
	}
}

func shutDownContainer(client *client, c *Container, heartbeat *agent.Heartbeat) {
	heartbeat.Status = "DOWN"

	if c.err != nil {
		heartbeat.Err = c.err.Error()
		heartbeat.ContainerProcessStatus = agent.ContainerProcessStatus{}
	}

	// The container process has exited and will not be restarted; send
	// heartbeats to the agent until it replies with a terminal state (DOWN or
	// FORCEDOWN).
	for {
		want, err := client.sendHeartbeat(*heartbeat)
		if err != nil {
			log.Println("unable to reach host agent: ", err)

			time.Sleep(time.Second)
			continue
		}

		if want == "DOWN" || want == "FORCEDOWN" {
			return
		}
	}
}
