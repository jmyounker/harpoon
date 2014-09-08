package main

import (
	"encoding/json"
	"net"
	"syscall"
	"time"

	"github.com/bernerdschaefer/eventsource"
)

type controller struct {
	ln net.Listener
	s  Supervisor
}

func newController(ln net.Listener, s Supervisor) *controller {
	return &controller{
		ln: ln,
		s:  s,
	}
}

// Run accepts and serves controller connections until the supervisor exits.
func (c *controller) Run() {
	exited := c.s.Exited()

	go func() {
		<-exited

		c.ln.Close()
	}()

	for {
		conn, err := c.ln.Accept()

		if err != nil {
			select {
			case <-exited:
				return
			case <-time.After(time.Second):
				continue
			}
		}

		c := newControllerConn(conn, c.s)
		go c.serve()
	}
}

type controllerConn struct {
	conn   net.Conn
	s      Supervisor
	writec chan ContainerProcessState
}

func newControllerConn(conn net.Conn, s Supervisor) *controllerConn {
	return &controllerConn{
		conn:   conn,
		s:      s,
		writec: make(chan ContainerProcessState),
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
		statec = make(chan ContainerProcessState)

		state  ContainerProcessState // last state notification
		writec chan ContainerProcessState
	)

	c.s.Notify(statec)
	defer c.s.Unnotify(statec)

	defer c.conn.Close()
	defer close(closed)

	go func() { errc <- c.readLoop() }()
	go func() { errc <- c.writeLoop(closed) }()

	for {
		select {
		case <-errc:
			return // TODO: do something with error?

		case <-exited:
			return

		case state = <-statec:
			writec = c.writec

		case writec <- state:
			writec = nil

		}
	}
}
