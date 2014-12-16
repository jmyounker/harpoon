package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/bernerdschaefer/eventsource"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestController(t *testing.T) {
	var (
		s     = newTestSupervisor()
		ln, _ = net.Listen("tcp", ":0")
		addr  = ln.Addr().String()
		c     = newController(ln, s)

		done = make(chan struct{})
	)

	defer ln.Close()

	go func() {
		c.Run()
		done <- struct{}{}
	}()

	<-time.After(5 * time.Second)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal("unable to dial: ", err)
	}

	var (
		enc = eventsource.NewEncoder(conn)
	)

	var subscribe chan<- agent.ContainerProcessState
	select {
	case subscribe = <-s.subscribec:
	case <-time.After(time.Millisecond):
		panic("client connection did not subscribe to supervisor")
	}

	select {
	case subscribe <- agent.ContainerProcessState{Up: true}:
	case <-time.After(time.Millisecond):
		panic("unable to send state to subscriber")
	}

	state, err := readStateEvent(conn)
	if err != nil {
		t.Fatal("error reading state event: ", err)
	}

	if state != (agent.ContainerProcessState{Up: true}) {
		t.Fatalf("unexpected state %#v", state)
	}

	if err := enc.Encode(eventsource.Event{Type: "stop"}); err != nil {
		t.Fatal("error sending stop command: ", err)
	}

	select {
	case stop := <-s.stopc:
		if stop != syscall.SIGTERM {
			t.Fatal("expected SIGTERM, got ", stop)
		}
	case <-time.After(time.Millisecond):
		panic("client connection did call stop on supervisor")
	}

	// supervisor reports down state
	select {
	case subscribe <- agent.ContainerProcessState{Up: false, Restarting: false}:
	case <-time.After(time.Millisecond):
		panic("unable to send state to subscriber")
	}

	state, err = readStateEvent(conn)
	if err != nil {
		t.Fatal("error reading state event: ", err)
	}

	if state != (agent.ContainerProcessState{Up: false, Restarting: false}) {
		t.Fatalf("unexpected state %#v", state)
	}

	if err := enc.Encode(eventsource.Event{Type: "exit"}); err != nil {
		t.Fatal("error sending exit command: ", err)
	}

	// controller should call Exit
	select {
	case <-s.exitc:
	case <-time.After(time.Millisecond):
		panic("controller connection did not call exit on supervisor")
	}

	// supervisor exits
	close(s.exited)

	// user connection is removed
	select {
	case <-s.unsubscribec:
	case <-time.After(time.Millisecond):
		panic("controller connection did not unsubscribe after supervisor exit")
	}

	// controller terminates
	select {
	case <-done:
	case <-time.After(time.Millisecond):
		panic("controller did not terminate after supervisor exit")
	}
}

func readStateEvent(r io.Reader) (agent.ContainerProcessState, error) {
	var (
		ev    eventsource.Event
		state agent.ContainerProcessState
	)

	if err := eventsource.NewDecoder(r).Decode(&ev); err != nil {
		return agent.ContainerProcessState{}, err
	}

	if ev.Type != "state" {
		return agent.ContainerProcessState{}, fmt.Errorf("unexpected event type %s", ev.Type)
	}

	if err := json.Unmarshal(ev.Data, &state); err != nil {
		return agent.ContainerProcessState{}, err
	}

	return state, nil
}
