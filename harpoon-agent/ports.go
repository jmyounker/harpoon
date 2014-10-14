package main

import (
	"errors"
	"fmt"
	"net"
)

// portDB manages all ports, both statically and dynamically allocated.
//
// It provides threadsafe and atomic operations.  All ports are claimed, acquired,
// or released atomically.
type portDB struct {
	ports        map[uint16]struct{} // set of static ports
	dynamicRange *portRange          // portRange operates on dynamicPorts

	acquirePortsc chan acquirePortCmd
	claimPortsc   chan acquirePortCmd
	releasePortsc chan acquirePortCmd
	exitc         chan chan struct{}
}

type acquirePortCmd struct {
	ports map[string]uint16
	errc  chan error
}

func newPortDB(startPort, endPort uint16) *portDB {
	ports := map[uint16]struct{}{}
	pdb := &portDB{
		ports:        ports,
		dynamicRange: newPortRange(startPort, endPort),

		acquirePortsc: make(chan acquirePortCmd),
		claimPortsc:   make(chan acquirePortCmd),
		releasePortsc: make(chan acquirePortCmd),
		exitc:         make(chan chan struct{}),
	}

	go pdb.loop()

	return pdb
}

// acquirePorts atomically claims static ports and chooses dynamic ports as a group.
//
// Static ports cannot be within the dynamic port range.  Dynamic ports are
// specified by setting the port value to zero.  If the call returns successfully
// the returned map will have all zeros replaced with port numbers.
//
// The ports acquired in this call, both static and dynamic, will not be available for
// use by other callers until they are freed using the releasePorts() call.
//
// If this operation fails, no ports have been claimed.
//
// The map 'ports' is updated in-place, replacing zero values with the reserved port
// number.
func (pdb *portDB) acquirePorts(ports map[string]uint16) error {
	errc := make(chan error)
	pdb.acquirePortsc <- acquirePortCmd{ports: ports, errc: errc}
	return <-errc
}

// claimPorts atomically claims ports from the appropriate pool.
//
// If a port is within the dynamic range it will be claimed from the dynamic port set,
// and otherwise it is claimed from the static port set. Attempts to claim a zero port
// cause this operation to fail.
//
// The ports acquired in this call, both static and dynamic, will not be available for
// use by other callers until they are freed using the releasePorts() call.
//
// If this operation fails, no ports have been claimed.
func (pdb *portDB) claimPorts(ports map[string]uint16) error {
	errc := make(chan error)
	pdb.claimPortsc <- acquirePortCmd{ports: ports, errc: errc}
	return <-errc
}

// releasePorts atomically returns ports to the appropriate pool.
//
// No checking is done, so an evil programmer could release ports belonging to
// other applications and screw everything up.
func (pdb *portDB) releasePorts(ports map[string]uint16) {
	errc := make(chan error)
	pdb.releasePortsc <- acquirePortCmd{ports: ports, errc: errc}
	<-errc
}

func (pdb *portDB) exit() {
	exitc := make(chan struct{})
	pdb.exitc <- exitc
	<-exitc
}

func (pdb *portDB) loop() {
	for {
		select {
		case cmd := <-pdb.acquirePortsc:
			cmd.errc <- pdb.acquirePortsUnsafe(cmd.ports)
		case cmd := <-pdb.claimPortsc:
			cmd.errc <- pdb.claimPortsUnsafe(cmd.ports)
		case cmd := <-pdb.releasePortsc:
			pdb.releasePortsUnsafe(cmd.ports)
			close(cmd.errc)
		case exitc := <-pdb.exitc:
			close(exitc)
			return
		}
	}
}

// acquirePortsUnsafe has the same interfaces as acquirePorts, destructively
// replacing port values of zero in the 'ports' map with allocated port numbers.
func (pdb *portDB) acquirePortsUnsafe(ports map[string]uint16) error {
	staticPorts := map[uint16]struct{}{}
	for _, port := range ports {
		if port != 0 {
			staticPorts[port] = struct{}{}
		}
	}

	// Check if the requested ports are available. If this passes then the sets are
	// mutually exclusive.
	if areSetsIntersecting(pdb.ports, staticPorts) {
		return errors.New("at least one static port already claimed")
	}
	if anyPortsInUse(staticPorts) {
		return errors.New("at least one static port already in use")
	}

	// Marks all statically allocated ports as being claimed. This must be done before
	// attempting dynamic allocation, because dynamic allocation depends on knowing if
	// any ports in the range it manages were explicitly allocated.
	setUnionInto(pdb.ports, staticPorts)

	// Chooses the dynamic set of ports. This happens atomically, with no update to 'ports',
	// so there's no need for cleanup 'ports'.
	err := pdb.dynamicRange.choosePorts(pdb.ports, ports)
	// However if it fails, we do need to undo the static allocations we made.  We can do this
	// because we ensure the port sets are mutually exclusive.
	if err != nil {
		setSubtractInto(pdb.ports, staticPorts)
		return err
	}
	return nil
}

func (pdb *portDB) claimPortsUnsafe(ports map[string]uint16) error {
	// Get set of ports
	portSet := map[uint16]struct{}{}
	for _, port := range ports {
		portSet[port] = struct{}{}
	}

	// Check if ports are available for claiming
	if areSetsIntersecting(pdb.ports, portSet) {
		return errors.New("at least one static port already claimed")
	}

	// Previous guard should prevent double-dipping from the port bowl
	setUnionInto(pdb.ports, portSet)

	return nil
}

func (pdb *portDB) releasePortsUnsafe(ports map[string]uint16) {
	// Get set of ports
	portSet := map[uint16]struct{}{}
	for _, port := range ports {
		portSet[port] = struct{}{}
	}

	setSubtractInto(pdb.ports, portSet)
}

// portRange manages a range of dynamically allocated ports.
//
// This structure is concurrency unsafe.
type portRange struct {
	startPort uint16 // inclusive
	endPort   uint16 // inclusive
	port      uint16 // this will be the next port returned by nextPort
}

// newPortRange gets a portRange for a specified range, and starts servicing requests.
func newPortRange(startPort, endPort uint16) *portRange {
	return &portRange{
		startPort: startPort,
		endPort:   endPort,
		port:      startPort,
	}
}

// choosePorts chooses ports.
//
// Ports which are set to zero will have their values replaced by randomly chosen ports
// from the range.  The 'ports' struct is updated in place.  All ports will be claimed or
// none will be claimed.
func (pr *portRange) choosePorts(claimedPorts map[uint16]struct{}, ports map[string]uint16) error {
	// Pick out a set of randomly assigned ports for those that are zero in a separate
	// step in case allocation fails.
	chosenPorts := map[string]uint16{}
	for name, assignedPort := range ports {
		if assignedPort != 0 {
			continue
		}
		port, err := pr.choosePort(claimedPorts)
		if err != nil {
			// Choose port marks the port as claimed in the port set, so it is necessary
			// to release the ports it claimed. This ensures the appearance of atomic
			// allocation.
			for _, port := range chosenPorts {
				delete(claimedPorts, port)
			}
			return err
		}
		chosenPorts[name] = port
	}

	// All ports have been successfully selected. Now we can update the
	// wider state without fear of failure.
	for name, port := range chosenPorts {
		ports[name] = port
	}

	return nil
}

// choosePort chooses a single port.
func (pr *portRange) choosePort(claimedPorts map[uint16]struct{}) (uint16, error) {
	// It's an inclusive range, so we add one
	numberPorts := int(pr.endPort) - int(pr.startPort) + 1
	// We might have to conceivably go through the entire range to find the one
	// remaining port.
	for i := 0; i < numberPorts; i++ {
		port := pr.nextPort()
		_, assigned := claimedPorts[port]
		if assigned {
			continue
		}

		if isPortInUse(port) {
			continue
		}

		claimedPorts[port] = struct{}{}

		return port, nil
	}
	return 0, fmt.Errorf("could not allocate a port within %d attempts", numberPorts)
}

// nextPort returns the next port in the range and then calculates and records its successor.
func (pr *portRange) nextPort() uint16 {
	nextPort := pr.port
	port := pr.port + 1
	if port > pr.endPort {
		port = pr.startPort
	}
	pr.port = port
	return nextPort
}

// anyPortsInUse returns true if something is listening on any on of the requested ports.
func anyPortsInUse(ports map[uint16]struct{}) bool {
	for port := range ports {
		if isPortInUse(port) {
			return true
		}
	}
	return false
}

// isPortInUse returns true if the port is not available for binding.
func isPortInUse(port uint16) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return true
	}
	ln.Close()
	return false
}

// areSetsIntersecting returns true if b intersects with a.
//
// Set b is assumed to be smaller, so is used to check.
func areSetsIntersecting(seta map[uint16]struct{}, setb map[uint16]struct{}) bool {
	for b := range setb {
		if _, ok := seta[b]; ok {
			return true
		}
	}
	return false
}

// setUnion destructively computes the union of dest and src.
//
// The result is left in dest.
func setUnionInto(dest map[uint16]struct{}, src map[uint16]struct{}) {
	for x := range src {
		dest[x] = struct{}{}
	}
}

// setSubtractInto destructively computes dest - src.
//
// The result is left in dest.
func setSubtractInto(dest map[uint16]struct{}, src map[uint16]struct{}) {
	for x := range src {
		delete(dest, x)
	}
}
