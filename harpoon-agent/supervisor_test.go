package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/bernerdschaefer/eventsource"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestSupervisorConnectEarlyExit(t *testing.T) {
	var (
		s         = &supervisor{}
		errExited = fmt.Errorf("exited")
		exitedc   = make(chan error, 1)
	)

	exitedc <- errExited

	if _, err := s.connect("/tmp/noexist", exitedc); err != errExited {
		t.Fatalf("expected connect to return errExited, got %v", err)
	}
}

func TestSupervisorConnectRetry(t *testing.T) {
	var (
		s     = &supervisor{}
		errc  = make(chan error)
		connc = make(chan io.ReadWriteCloser)
	)

	tmpdir, err := ioutil.TempDir("", "harpoon-agent-")
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(tmpdir)
	controlPath := tmpdir + "/control"

	go func() {
		rwc, err := s.connect(controlPath, nil)

		if err != nil {
			errc <- err
			return
		}

		connc <- rwc
	}()

	select {
	case err := <-errc:
		t.Fatalf("expected no error, got %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	ln, err := net.Listen("unix", controlPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	select {
	case err := <-errc:
		t.Fatalf("expected no error, got %v", err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("supervisor did not connect after control socket was created")
	case rwc := <-connc:
		rwc.Close()
	}
}

func TestSupervisorConnectRetryListenRace(t *testing.T) {
	var (
		s     = &supervisor{}
		errc  = make(chan error)
		connc = make(chan io.ReadWriteCloser)
	)

	tmpdir, err := ioutil.TempDir("", "harpoon-agent-")
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(tmpdir)
	controlPath := tmpdir + "/control"

	// set up control socket, but don't listen
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal("unable to create socket: ", err)
	}
	defer syscall.Close(fd)

	if err := syscall.Bind(fd, &syscall.SockaddrUnix{Name: controlPath}); err != nil {
		t.Fatal("unable to bind control socket: ", err)
	}

	go func() {
		rwc, err := s.connect(controlPath, nil)

		if err != nil {
			errc <- err
			return
		}

		connc <- rwc
	}()

	// let the agent attempt to connect a few times
	select {
	case err := <-errc:
		t.Fatalf("expected no error, got %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	if err := syscall.Listen(fd, 1); err != nil {
		t.Fatal("unable to listen on control socket: ", err)
	}

	select {
	case err := <-errc:
		t.Fatalf("expected no error, got %v", err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("supervisor did not connect after control socket was created")
	case rwc := <-connc:
		rwc.Close()
	}
}

func TestSupervisorConnectDeadSupervisor(t *testing.T) {
	var (
		s     = &supervisor{}
		errc  = make(chan error)
		connc = make(chan io.ReadWriteCloser)
	)

	tmpdir, err := ioutil.TempDir("", "harpoon-agent-")
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(tmpdir)
	controlPath := tmpdir + "/control"

	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal("unable to create socket: ", err)
	}
	defer syscall.Close(fd)

	if err := syscall.Bind(fd, &syscall.SockaddrUnix{Name: controlPath}); err != nil {
		t.Fatal("unable to bind control socket: ", err)
	}

	go func() {
		rwc, err := s.connect(controlPath, nil)

		if err != nil {
			errc <- err
			return
		}

		connc <- rwc
	}()

	select {
	case err := <-errc:
		t.Fatalf("expected no error, got %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	select {
	case <-errc:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("supervisor did not time out connecting to dead control socket")
	}
}

func TestSupervisor(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "harpoon-agent-")
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(tmpdir)

	controlPath := tmpdir + "/control"

	var (
		s        = newSupervisor(tmpdir)
		done     = make(chan struct{})
		exitErrc = make(chan error)
	)

	ln, err := net.Listen("unix", controlPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		rwc, err := s.connect(controlPath, nil)
		if err != nil {
			panic(err)
		}

		s.loop(rwc)

		done <- struct{}{}
	}()

	// give the supervisor up to 100ms to connect
	ln.(*net.UnixListener).SetDeadline(time.Now().Add(100 * time.Millisecond))

	conn, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	var statec = make(chan agent.ContainerProcessState)
	s.Subscribe(statec)

	go func() { exitErrc <- s.Exit() }()

	select {
	case err := <-exitErrc:
		if err == nil {
			t.Fatal("Exit expected to return error before first state received")
		}
	case <-time.After(time.Millisecond):
		panic("exit before down did not return")
	}

	// supervisor process reports container is running
	sendControlState(t, conn, agent.ContainerProcessState{Up: true}, time.Millisecond)

	select {
	case state := <-statec:
		if !state.Up {
			t.Fatal("expected notification that container was running")
		}
	case <-time.After(time.Millisecond):
		panic("no state sent")
	}

	s.Unsubscribe(statec)

	// test subscribe after state has been received
	s.Subscribe(statec)

	select {
	case state := <-statec:
		if !state.Up {
			t.Fatal("expected subscribe to immediately receive the latest state")
		}
	case <-time.After(time.Millisecond):
		panic("no state sent")
	}

	s.Stop(time.Second)

	if ev := receiveControlEvent(t, conn, time.Millisecond); ev != "stop" {
		t.Fatalf("expected stop event, got %q", ev)
	}

	go func() { exitErrc <- s.Exit() }()

	select {
	case err := <-exitErrc:
		if err == nil {
			t.Fatal("Exit expected to return error when container is running")
		}
	case <-time.After(time.Millisecond):
		panic("exit before down did not return")
	}

	// supervisor reports container is stopped
	sendControlState(
		t,
		conn,
		agent.ContainerProcessState{Up: false, Restarting: false},
		time.Millisecond,
	)

	select {
	case state := <-statec:
		if state.Up {
			t.Fatal("expected notification that container was stopped")
		}
	case <-time.After(time.Millisecond):
		panic("no state sent")
	}

	go func() { exitErrc <- s.Exit() }()

	if ev := receiveControlEvent(t, conn, 15*time.Millisecond); ev != "exit" {
		t.Fatalf("expected exit event, got %q", ev)
	}

	conn.Close()
	ln.Close()

	select {
	case err := <-exitErrc:
		if err != nil {
			t.Fatal("expected Exit to succeed, got: ", err)
		}
	case <-time.After(time.Millisecond):
		panic("Exit call did not return")
	}

	select {
	case <-done:
	case <-time.After(time.Millisecond):
		panic("supervisor loop did not shut down after exit")
	}

	unsubscribed := make(chan struct{})
	go func() { s.Unsubscribe(statec); unsubscribed <- struct{}{} }()

	select {
	case <-unsubscribed:
	case <-time.After(time.Millisecond):
		panic("unable to unsubscribe after exit")
	}
}

func TestSupervisorKillAfterGrace(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "harpoon-agent-")
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(tmpdir)

	controlPath := tmpdir + "/control"

	var (
		s        = newSupervisor(tmpdir)
		done     = make(chan struct{})
		exitErrc = make(chan error)
	)

	ln, err := net.Listen("unix", controlPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		rwc, err := s.connect(controlPath, nil)
		if err != nil {
			panic(err)
		}

		s.loop(rwc)

		done <- struct{}{}
	}()

	// give the supervisor up to 100ms to connect
	ln.(*net.UnixListener).SetDeadline(time.Now().Add(100 * time.Millisecond))

	conn, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	var statec = make(chan agent.ContainerProcessState)
	s.Subscribe(statec)

	// supervisor process reports container is running
	sendControlState(t, conn, agent.ContainerProcessState{Up: true}, time.Millisecond)

	select {
	case state := <-statec:
		if !state.Up {
			t.Fatal("expected notification that container was running")
		}
	case <-time.After(time.Millisecond):
		panic("no state sent")
	}

	// stop w/ 10ms grace
	s.Stop(10 * time.Millisecond)

	if ev := receiveControlEvent(t, conn, time.Millisecond); ev != "stop" {
		t.Fatalf("expected stop event, got %q", ev)
	}

	if ev := receiveControlEvent(t, conn, 15*time.Millisecond); ev != "kill" {
		t.Fatalf("expected kill event, got %q", ev)
	}

	// send terminal state update
	sendControlState(
		t,
		conn,
		agent.ContainerProcessState{Up: false, Restarting: false},
		time.Millisecond,
	)

	select {
	case state := <-statec:
		if state.Up {
			t.Fatal("expected notification that container was stopped")
		}
	case <-time.After(time.Millisecond):
		panic("no state sent")
	}

	go func() { exitErrc <- s.Exit() }()

	if ev := receiveControlEvent(t, conn, 15*time.Millisecond); ev != "exit" {
		t.Fatalf("expected exit event, got %q", ev)
	}

	conn.Close()
	ln.Close()

	select {
	case err := <-exitErrc:
		if err != nil {
			t.Fatal("expected Exit to succeed, got: ", err)
		}
	case <-time.After(time.Millisecond):
		panic("Exit call did not return")
	}

	select {
	case <-done:
	case <-time.After(time.Millisecond):
		panic("supervisor loop did not shut down after exit")
	}
}

func sendControlState(t *testing.T, conn net.Conn, state agent.ContainerProcessState, d time.Duration) {
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal("unable to marshal state: ", err)
	}

	conn.SetDeadline(time.Now().Add(d))

	err = eventsource.NewEncoder(conn).Encode(eventsource.Event{
		Type: "state",
		Data: data,
	})

	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatalf("timeout sending state after %s", d)
	}

	if err != nil {
		t.Fatal("unable to send state: ", err)
	}
}

func receiveControlEvent(t *testing.T, conn net.Conn, d time.Duration) string {
	var event eventsource.Event

	conn.SetDeadline(time.Now().Add(d))

	if err := eventsource.NewDecoder(conn).Decode(&event); err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			t.Fatalf("timeout receiving event after %s", d)
		}

		t.Fatal("unable to receive event: ", err)
	}

	return event.Type
}
