package main

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"syscall"
	"time"

	"github.com/bernerdschaefer/eventsource"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type controller struct {
	net.Listener
	Supervisor
}

func newController(ln net.Listener, s Supervisor) *controller {
	return &controller{
		Listener:   ln,
		Supervisor: s,
	}
}

// Run accepts and serves controller connections until the supervisor exits.
func (c *controller) Run() {
	exited := c.Supervisor.Exited()

	go func() {
		<-exited

		c.Listener.Close()
	}()

	for {
		conn, err := c.Listener.Accept()

		if err != nil {
			select {
			case <-exited:
				return
			case <-time.After(time.Second):
				continue
			}
		}

		c := newControllerConn(conn, c.Supervisor)
		go c.serve()
	}
}

type controllerConn struct {
	conn   net.Conn
	s      Supervisor
	writec chan agent.ContainerProcessState
}

func newControllerConn(conn net.Conn, s Supervisor) *controllerConn {
	return &controllerConn{
		conn:   conn,
		s:      s,
		writec: make(chan agent.ContainerProcessState),
	}
}

func (c *controllerConn) readLoop() error {
	dec := eventsource.NewDecoder(c.conn)

	for {
		var ev eventsource.Event

		if err := dec.Decode(&ev); err != nil {
			return err
		}

		switch ev.Type {
		case "stop":
			c.s.Stop(syscall.SIGTERM)
		case "kill":
			c.s.Stop(syscall.SIGKILL)
		case "exit":
			c.s.Exit()
		}
	}
}

func (c *controllerConn) writeLoop(closed chan struct{}) error {
	enc := eventsource.NewEncoder(c.conn)

	for {
		select {
		case <-closed:
			return nil

		case state := <-c.writec:
			buf, err := json.Marshal(state)
			if err != nil {
				return err
			}

			err = enc.Encode(eventsource.Event{
				Type: "state",
				Data: buf,
			})

			if err != nil {
				return err
			}
		}
	}
}

func (c *controllerConn) serve() {
	var (
		exited = c.s.Exited()
		errc   = make(chan error, 2)
		closed = make(chan struct{})
		statec = make(chan agent.ContainerProcessState)

		state  agent.ContainerProcessState // last state notification
		writec chan agent.ContainerProcessState
	)

	c.s.Subscribe(statec)
	defer c.s.Unsubscribe(statec)

	defer c.conn.Close()
	defer close(closed)

	go func() { errc <- c.readLoop() }()
	go func() { errc <- c.writeLoop(closed) }()

	for {
		select {
		case err := <-errc:
			if err != nil && err != io.EOF {
				log.Println("unexpected error on control connection: ", err)
			}

			return

		case <-exited:
			return

		case state = <-statec:
			writec = c.writec

		case writec <- state:
			writec = nil

		}
	}
}
