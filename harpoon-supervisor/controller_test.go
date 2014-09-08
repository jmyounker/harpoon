package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"

	"github.com/bernerdschaefer/eventsource"
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

	go func() { c.Run(); done <- struct{}{} }()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal("unable to dial: ", err)
	}

	var (
		enc = eventsource.NewEncoder(conn)
	)

	notify := <-s.notifyc
	notify <- ContainerProcessState{Up: true}

	state, err := readStateEvent(conn)
	if err != nil {
		t.Fatal("error reading state event: ", err)
	}

	if state != (ContainerProcessState{Up: true}) {
		t.Fatalf("unexpected state %#v", state)
	}

	if err := enc.Encode(eventsource.Event{Type: "stop"}); err != nil {
		t.Fatal("error sending stop command: ", err)
	}

	if stop := <-s.stopc; stop != syscall.SIGTERM {
		t.Fatal("expected SIGTERM, got ", stop)
	}

	// supervisor reports down state
	notify <- ContainerProcessState{Up: false, Restarting: false}

	state, err = readStateEvent(conn)
	if err != nil {
		t.Fatal("error reading state event: ", err)
	}

	if state != (ContainerProcessState{Up: false, Restarting: false}) {
		t.Fatalf("unexpected state %#v", state)
	}

	if err := enc.Encode(eventsource.Event{Type: "exit"}); err != nil {
		t.Fatal("error sending exit command: ", err)
	}

	// controller should call Exit
	<-s.exitc

	// supervisor exits
	close(s.exited)

	// user connection is removed
	<-s.unnotifyc

	// controller terminates
	<-done
}

func readStateEvent(r io.Reader) (ContainerProcessState, error) {
	var (
		ev    eventsource.Event
		state ContainerProcessState
	)

	if err := eventsource.NewDecoder(r).Decode(&ev); err != nil {
		return ContainerProcessState{}, err
	}

	if ev.Type != "state" {
		return ContainerProcessState{}, fmt.Errorf("unexpected event type %s", ev.Type)
	}

	if err := json.Unmarshal(ev.Data, &state); err != nil {
		return ContainerProcessState{}, err
	}

	return state, nil
}
