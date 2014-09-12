package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"path"
	"syscall"
	"time"

	"github.com/bernerdschaefer/eventsource"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type supervisor struct {
	rundir string

	exitc        chan chan error
	stopc        chan time.Duration
	subscribec   chan chan<- agent.ContainerProcessState
	unsubscribec chan chan<- agent.ContainerProcessState
	statec       chan agent.ContainerProcessState

	exited chan struct{}
}

func newSupervisor(rundir string) *supervisor {
	return &supervisor{
		rundir:       rundir,
		exitc:        make(chan chan error),
		stopc:        make(chan time.Duration),
		subscribec:   make(chan chan<- agent.ContainerProcessState),
		unsubscribec: make(chan chan<- agent.ContainerProcessState),
		statec:       make(chan agent.ContainerProcessState),
		exited:       make(chan struct{}),
	}
}

// Start starts the supervisor and connects to its control socket. If an error
// is returned, the supervisor was not started.
func (s *supervisor) Start(config agent.ContainerConfig, stdout, stderr io.Writer) error {
	cmd := exec.Command("harpoon-supervisor", config.Command.Exec...)

	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Dir = s.rundir

	if err := cmd.Start(); err != nil {
		return err
	}

	exitedc := make(chan error, 1)

	// Wait on the command to prevent zombies, and send exit status on exitedc in
	// case it exits before we can connect.
	go func() { exitedc <- cmd.Wait() }()

	rwc, err := s.connect(path.Join(s.rundir, "control"), exitedc)
	if err != nil {
		return err
	}

	go s.loop(rwc)

	return nil
}

// connect waits for controlPath to exist and be connectable, or an error to be
// sent on exitedc.
func (s *supervisor) connect(controlPath string, exitedc chan error) (io.ReadWriteCloser, error) {
	for {
		conn, err := net.Dial("unix", controlPath)

		if err == nil {
			return conn, nil
		}

		select {
		case err = <-exitedc:
			return nil, err
		default:
		}

		ne, ok := err.(*net.OpError)

		if !ok {
			return nil, err
		}

		if ne.Err != syscall.ENOENT {
			return nil, err
		}

		select {
		case err = <-exitedc:
			return nil, err
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Stop shuts down the supervisor. If the container does not exit gracefully
// before the grace period, it will be forcefully killed.
func (s *supervisor) Stop(grace time.Duration) {
	s.stopc <- grace
}

func (s *supervisor) Subscribe(c chan<- agent.ContainerProcessState) {
	s.subscribec <- c
}

func (s *supervisor) Unsubscribe(c chan<- agent.ContainerProcessState) {
	select {
	case s.unsubscribec <- c:
	case <-s.exited:
	}
}

func (s *supervisor) readLoop(r io.Reader) error {
	dec := eventsource.NewDecoder(r)

	for {
		var event eventsource.Event

		if err := dec.Decode(&event); err != nil {
			return err
		}

		// Ignore unknown events
		if event.Type != "state" {
			continue
		}

		var state agent.ContainerProcessState

		if err := json.Unmarshal(event.Data, &state); err != nil {
			return err
		}

		s.statec <- state
	}
}

// Exit instructs the supervisor to exit. If the supervisor is not already
// stopped, it will return an error.
func (s *supervisor) Exit() error {
	errc := make(chan error, 1)

	select {
	case <-s.exited:
		return nil
	case s.exitc <- errc:
	}

	if err := <-errc; err != nil {
		return err
	}

	<-s.exited
	return nil
}

func (s *supervisor) loop(rwc io.ReadWriteCloser) {
	var (
		errc        = make(chan error, 1)
		subscribers = map[chan<- agent.ContainerProcessState]struct{}{}
		killTimer   <-chan time.Time

		lastState *agent.ContainerProcessState
	)

	defer close(s.exited)
	defer rwc.Close()

	defer func() {
		if killTimer != nil {
			incContainerStatusForceDownSuccessful(1)
			return
		}

		incContainerStatusDownSuccessful(1)
	}()

	go func() { errc <- s.readLoop(rwc) }()

	for {
		select {
		case err := <-errc:
			if err != nil && err != io.EOF {
				log.Println("unexpected error on control connection: ", err)
			}

			return

		case c := <-s.exitc:
			if lastState == nil {
				c <- fmt.Errorf("cannot exit from unknown state")
				continue
			}

			if lastState.Up || lastState.Restarting {
				c <- fmt.Errorf("supervisor not stopped")
				continue
			}

			err := eventsource.NewEncoder(rwc).Encode(eventsource.Event{
				Type: "exit",
			})

			c <- err

		case state := <-s.statec:
			lastState = &state

			for c := range subscribers {
				c <- state
			}

		case c := <-s.subscribec:
			subscribers[c] = struct{}{}

			if lastState != nil {
				c <- *lastState
			}

		case c := <-s.unsubscribec:
			delete(subscribers, c)

		case grace := <-s.stopc:
			eventsource.NewEncoder(rwc).Encode(eventsource.Event{
				Type: "stop",
			})

			killTimer = time.After(grace)

		case <-killTimer:
			incContainerStatusKilled(1)

			eventsource.NewEncoder(rwc).Encode(eventsource.Event{
				Type: "kill",
			})
		}
	}
}
