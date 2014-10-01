package main

import (
	"errors"
	"fmt"
	"net"
)

// portRange manages a range of ports.
type portRange struct {
	startPort uint16 // inclusive
	endPort   uint16 // inclusive
	ports     map[uint16]struct{}

	// The next port that might be available.
	portc chan uint16

	// Sent from GetPort to getPort. Contains a channel for getPort to back results.
	getportc chan chan getPortResults

	// Sent from ReturnPort. Contains the port to return to the pool.
	returnportc chan uint16

	// Closed to indicate that goroutines should terminate.
	exitc chan struct{}
}

// getPortResults communicates results back from the getPort call to the GetPort call.
type getPortResults struct {
	port uint16
	err  error
}

// newPortRange gets a portRange for a specified range, and starts servicing requests.
func newPortRange(startPort, endPort uint16) *portRange {
	pr := &portRange{
		startPort: startPort,
		endPort:   endPort,
		ports:     map[uint16]struct{}{},

		getportc:    make(chan chan getPortResults),
		returnportc: make(chan uint16),
		exitc:       make(chan struct{}),
	}
	pr.portc = pr.possiblePorts()
	go pr.loop()
	return pr
}

// getPort picks an unallocated port from the range, while making sure that the port is not used.
func (pr *portRange) getPort() (uint16, error) {
	resultc := make(chan getPortResults)
	pr.getportc <- resultc
	result := <-resultc
	return result.port, result.err
}

// returnPort returns a used port to the range. Returning a port which has not been
// marked as used is not an error.
func (pr *portRange) returnPort(port uint16) {
	pr.returnportc <- port
}

// exit terminates the loop() function.
func (pr *portRange) exit() {
	close(pr.exitc)
}

// getPort implements GetPort functionality in a thread-hostile way.
func (pr *portRange) getPortUnsafe() (uint16, error) {
	// It's an inclusive range, so we add one
	numberPorts := int(pr.endPort) - int(pr.startPort) + 1
	if len(pr.ports) >= numberPorts {
		return 0, errors.New("all ports are allocated")
	}
	maxAttempts := numberPorts
	for i := 0; i < maxAttempts; i++ {
		port := <-pr.portc
		_, assigned := pr.ports[port]
		if assigned {
			continue
		}

		if pr.isPortInUse(port) {
			continue
		}

		pr.ports[port] = struct{}{}
		return port, nil
	}
	return 0, fmt.Errorf("could not allocate a port within %d attempts", maxAttempts)
}

// loop coordinates commands
func (pr *portRange) loop() {
	for {
		select {
		case resultc := <-pr.getportc:
			port, err := pr.getPortUnsafe()
			resultc <- getPortResults{port: port, err: err}
		case port := <-pr.returnportc:
			delete(pr.ports, port)
		case <-pr.exitc:
			return
		}
	}
}

// possiblePorts generates an infinite list of ports
//
// These ports are chosen from the range [startPort, endPort]. In this
// case they are enumerated from startPort up to endPort by one, and
// when the range is complete it wraps around to startPort.
func (pr *portRange) possiblePorts() chan uint16 {
	portc := make(chan uint16)
	go func() {
		for {
			for port := pr.startPort; port <= pr.endPort; port++ {
				select {
				case portc <- port:
				case <-pr.exitc:
					return
				}
			}
		}
	}()
	return portc
}

// isPortInUse returns true if the port is not available for binding.
func (pr *portRange) isPortInUse(port uint16) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return true
	}
	ln.Close()
	return false
}
