package main

import (
	"fmt"
	"net"
	"testing"
)

func TestGetPortSanely(t *testing.T) {
	pr := newPortRange(lowTestPort, highTestPort)
	defer pr.exit()
	port, err := pr.getPort()
	AssertNoError(t, err)
	if port < lowTestPort || port > highTestPort {
		t.Errorf("Port is %d out of range", port)
	}
}

func TestGetPortRecordsAllocation(t *testing.T) {
	pr := newPortRange(lowTestPort, highTestPort)
	defer pr.exit()
	port, err := pr.getPort()
	AssertNoError(t, err)
	if _, ok := pr.ports[port]; !ok {
		t.Error("Port allocation was not recorded")
	}
}

func TestReturnPortRemovesAllocation(t *testing.T) {
	pr := newPortRange(lowTestPort, highTestPort)
	defer pr.exit()
	port, err := pr.getPort()
	AssertNoError(t, err)
	pr.returnPort(port)
	if _, ok := pr.ports[port]; ok {
		t.Error("Port allocation was not returned")
	}
}

func TestCantGetAPortFromAFullyAllocatedRange(t *testing.T) {
	pr := newPortRange(lowTestPort, lowTestPort+2)
	defer pr.exit()
	pr.ports[lowTestPort] = struct{}{}
	pr.ports[lowTestPort+1] = struct{}{}
	pr.ports[lowTestPort+2] = struct{}{}
	_, err := pr.getPort()
	if err == nil {
		t.Error("expected error")
	}
}

func TestWorksWithOnePort(t *testing.T) {
	pr := newPortRange(lowTestPort, lowTestPort)
	defer pr.exit()
	port, err := pr.getPort()
	AssertNoError(t, err)
	if port != lowTestPort {
		t.Errorf("expected port %d but got port %d", port, lowTestPort)
	}
}

func TestCantGetAPortThatsAllocated(t *testing.T) {
	pr := newPortRange(lowTestPort, lowTestPort)
	defer pr.exit()
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", lowTestPort))
	if err != nil {
		t.Errorf("could not take over network port %d for tests", lowTestPort)
	}
	defer ln.Close()
	if _, err = pr.getPort(); err == nil {
		t.Error("expected error")
	}
}

func AssertNoError(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
}
