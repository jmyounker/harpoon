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
		ts       = newTestSupervisor(controlPath)
		done     = make(chan struct{})
		exitErrc = make(chan error)
	)

	go ts.run()

	go func() {
		rwc, err := s.connect(controlPath, nil)
		if err != nil {
			panic(err)
		}

		s.loop(rwc)

		done <- struct{}{}
	}()

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
	ts.statec <- agent.ContainerProcessState{Up: true}

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

	select {
	case <-ts.stopc:
	case <-time.After(time.Millisecond):
		panic("supervisor did not receive stop command")
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
	ts.statec <- agent.ContainerProcessState{Up: false, Restarting: false}

	select {
	case state := <-statec:
		if state.Up {
			t.Fatal("expected notification that container was stopped")
		}
	case <-time.After(time.Millisecond):
		panic("no state sent")
	}

	go func() { exitErrc <- s.Exit() }()

	select {
	case <-ts.exitc:
	case <-time.After(time.Millisecond):
		panic("supervisor did not receive exit command")
	}

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
		ts       = newTestSupervisor(controlPath)
		done     = make(chan struct{})
		exitErrc = make(chan error)
	)

	go ts.run()

	go func() {
		rwc, err := s.connect(controlPath, nil)
		if err != nil {
			panic(err)
		}

		s.loop(rwc)

		done <- struct{}{}
	}()

	var statec = make(chan agent.ContainerProcessState)
	s.Subscribe(statec)

	// supervisor process reports container is running
	ts.statec <- agent.ContainerProcessState{Up: true}

	select {
	case state := <-statec:
		if !state.Up {
			t.Fatal("expected notification that container was running")
		}
	case <-time.After(time.Millisecond):
		panic("no state sent")
	}

	s.Stop(10 * time.Millisecond)

	select {
	case <-ts.stopc:
	case <-time.After(time.Millisecond):
		panic("supervisor did not receive stop command")
	}

	select {
	case <-ts.killc:
	case <-time.After(15 * time.Millisecond):
		panic("supervisor did not receive kill command")
	}

	// supervisor reports container is stopped
	ts.statec <- agent.ContainerProcessState{Up: false, Restarting: false}

	select {
	case state := <-statec:
		if state.Up {
			t.Fatal("expected notification that container was stopped")
		}
	case <-time.After(time.Millisecond):
		panic("no state sent")
	}

	go func() { exitErrc <- s.Exit() }()

	select {
	case <-ts.exitc:
	case <-time.After(time.Millisecond):
		panic("supervisor did not receive exit command")
	}

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

// testSupervisor simulates the interface to harpoon-supervisor's control
// socket.
type testSupervisor struct {
	controlPath string

	statec chan agent.ContainerProcessState
	stopc  chan struct{}
	killc  chan struct{}
	exitc  chan struct{}
}

func newTestSupervisor(controlPath string) *testSupervisor {
	return &testSupervisor{
		controlPath: controlPath,

		statec: make(chan agent.ContainerProcessState),
		stopc:  make(chan struct{}),
		killc:  make(chan struct{}),
		exitc:  make(chan struct{}),
	}
}

// run accepts and processes a single connection
func (ts *testSupervisor) run() {
	ln, err := net.Listen("unix", ts.controlPath)
	if err != nil {
		panic(err)
	}
	defer ln.Close()

	conn, err := ln.Accept()
	if err != nil {
		panic(err)
	}

	defer conn.Close()
	defer close(ts.exitc)

	exit := make(chan struct{})

	go func() {
		dec := eventsource.NewDecoder(conn)

		for {
			var event eventsource.Event

			if err := dec.Decode(&event); err != nil {
				panic(err)
			}

			switch event.Type {
			case "stop":
				ts.stopc <- struct{}{}
			case "kill":
				ts.killc <- struct{}{}
			case "exit":
				close(exit)
				return
			}
		}
	}()

	enc := eventsource.NewEncoder(conn)

	for {
		select {
		case state := <-ts.statec:
			data, _ := json.Marshal(state)

			enc.Encode(eventsource.Event{
				Type: "state",
				Data: data,
			})

		case <-exit:
			return
		}
	}
}
