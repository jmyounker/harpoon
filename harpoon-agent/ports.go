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
	port      uint16 // this will be the next port returned by nextPort
	ports     map[uint16]struct{}

	// Sent from getPort to getPortUnsafe. Contains a channel for getPort to back results.
	getportc chan chan getPortResults

	// Set from claimPort. Passes in the port to claim.
	claimportc chan uint16

	// Sent from returnPort. Contains the port to return to the pool.
	returnportc chan uint16

	// Closed to indicate that goroutines should terminate.
	exitc chan chan struct{}
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
		port:      startPort,

		getportc:    make(chan chan getPortResults),
		claimportc:  make(chan uint16),
		returnportc: make(chan uint16),
		exitc:       make(chan chan struct{}),
	}
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
//
// Ensure this is called after the client goroutines have shut down.
func (pr *portRange) exit() {
	exitc := make(chan struct{})
	pr.exitc <- exitc
	<-exitc
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
		port := pr.nextPort()
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

// claimPort claims a port from the range without performing any checks
func (pr *portRange) claimPort(port uint16) {
	pr.claimportc <- port
}

// loop coordinates commands
func (pr *portRange) loop() {
	for {
		select {
		case resultc := <-pr.getportc:
			port, err := pr.getPortUnsafe()
			resultc <- getPortResults{port: port, err: err}
		case port := <-pr.claimportc:
			pr.ports[port] = struct{}{}
		case port := <-pr.returnportc:
			delete(pr.ports, port)
		case exitc := <-pr.exitc:
			close(exitc)
			return
		}
	}
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

// nextPort returns the next available port and then calculates and records its successor.
func (pr *portRange) nextPort() uint16 {
	nextPort := pr.port
	port := pr.port + 1
	if port > pr.endPort {
		port = pr.startPort
	}
	pr.port = port
	return nextPort
}
