package main

import (
	"fmt"
	"net"
	"reflect"
	"testing"
)

func TestAcquireDynamicPortSanely(t *testing.T) {
	pdb := newPortDB(lowTestPort, highTestPort)
	defer pdb.exit()
	ports := map[string]uint16{"p1": 0}

	err := pdb.acquirePorts(ports)

	AssertNoError(t, err)
	if len(ports) != 1 {
		t.Error("wrong number of ports acquired")
	}
	port := ports["p1"]
	if port < lowTestPort || port > highTestPort {
		t.Errorf("Port is %d out of range", port)
	}
}

func TestStaticPortSanely(t *testing.T) {
	pdb := newPortDB(lowTestPort, highTestPort)
	defer pdb.exit()
	p1 := lowTestPort - 2
	ports := map[string]uint16{"p1": p1}
	expected := map[string]uint16{"p1": p1}

	err := pdb.acquirePorts(ports)

	AssertNoError(t, err)
	if !reflect.DeepEqual(ports, expected) {
		t.Error("wrong port allocations received:", expected)
	}
}

func TestGetPortRecordsAllocation(t *testing.T) {
	pdb := newPortDB(lowTestPort, highTestPort)
	defer pdb.exit()
	p1 := lowTestPort - 2
	ports := map[string]uint16{"p1": p1}

	err := pdb.acquirePorts(ports)

	AssertNoError(t, err)
	if _, ok := pdb.ports[p1]; !ok {
		t.Error("Port allocation was not recorded")
	}
}

func TestReleasePortRemovesAllocation(t *testing.T) {
	pdb := newPortDB(lowTestPort, highTestPort)
	defer pdb.exit()
	p1 := lowTestPort - 2
	ports := map[string]uint16{"p1": p1}

	err := pdb.acquirePorts(ports)
	AssertNoError(t, err)

	pdb.releasePorts(ports)
	if _, ok := pdb.ports[p1]; ok {
		t.Error("Port allocation was not returned")
	}
}

func TestCantGetAPortFromAFullyAllocatedRange(t *testing.T) {
	pdb := newPortDB(lowTestPort, lowTestPort+2)
	defer pdb.exit()
	ports := map[string]uint16{"p1": 0, "p2": 0, "p3": 0, "p4": 0}

	err := pdb.acquirePorts(ports)

	if err == nil {
		t.Error("expected error")
	}
}

func TestDynamicAllocationFailureDoesNotAlterRequestedPorts(t *testing.T) {
	pdb := newPortDB(lowTestPort, lowTestPort+1)
	defer pdb.exit()
	p4 := lowTestPort - 2
	ports := map[string]uint16{"p1": 0, "p2": 0, "p3": 0, "p4": p4}

	pdb.acquirePorts(ports)

	expectedPorts := map[string]uint16{"p1": 0, "p2": 0, "p3": 0, "p4": p4}
	if !reflect.DeepEqual(ports, expectedPorts) {
		t.Error("expected no alterations but received", ports, "when expecting", expectedPorts)
	}
}

func TestCanGetPortFromOneElementRange(t *testing.T) {
	pdb := newPortDB(lowTestPort, lowTestPort)
	defer pdb.exit()
	ports := map[string]uint16{"p1": 0}

	err := pdb.acquirePorts(ports)

	if err != nil {
		t.Error("expected error")
	}
}

func TestCantGetAPortThatsAllocated(t *testing.T) {
	pdb := newPortDB(lowTestPort, lowTestPort)
	defer pdb.exit()
	ports := map[string]uint16{"p1": 0}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", lowTestPort))
	if err != nil {
		t.Errorf("could not take over network port %d for tests", lowTestPort)
	}
	defer ln.Close()
	if err := pdb.acquirePorts(ports); err == nil {
		t.Error("expected error")
	}
}

func TestClaimPort(t *testing.T) {
	pdb := newPortDB(lowTestPort, lowTestPort)
	defer pdb.exit()
	p1 := lowTestPort - 2
	ports := map[string]uint16{"p1": p1}
	pdb.claimPorts(ports)
	if _, ok := pdb.ports[p1]; !ok {
		t.Error("could not claim port")
	}
}

func TestStaticReacquisitionFailsWithoutAlteringClaimedPorts(t *testing.T) {
	pdb := newPortDB(lowTestPort, lowTestPort)
	defer pdb.exit()
	p1 := lowTestPort - 2
	p2 := lowTestPort - 1
	ports1 := map[string]uint16{"p1": p1}
	AssertNoError(t, pdb.claimPorts(ports1))

	ports2 := map[string]uint16{"p1": p1, "p2": p2}
	if err := pdb.acquirePorts(ports2); err == nil {
		t.Error("intersecting ports should have failed")
	}

	expectedClaimed := map[uint16]struct{}{p1: {}}
	if !reflect.DeepEqual(pdb.ports, expectedClaimed) {
		t.Error("wrong port allocations received:", pdb.ports, expectedClaimed)
	}
}

func TestDynamicReacquisitionFailsWithoutAlteringClaimedPorts(t *testing.T) {
	pdb := newPortDB(lowTestPort, lowTestPort)
	defer pdb.exit()
	ports1 := map[string]uint16{"p1": 0}
	AssertNoError(t, pdb.acquirePorts(ports1))

	p1 := ports1["p1"]
	ports2 := map[string]uint16{"p1": p1, "p2": 0}

	if err := pdb.acquirePorts(ports2); err == nil {
		t.Error("intersecting ports should have failed")
	}

	expectedClaimed := map[uint16]struct{}{p1: {}}
	if !reflect.DeepEqual(pdb.ports, expectedClaimed) {
		t.Error("wrong port allocations received:", pdb.ports, expectedClaimed)
	}
}

func TestReclaimingFailsWithoutAlteringClaimedPorts(t *testing.T) {
	pdb := newPortDB(lowTestPort, lowTestPort)
	defer pdb.exit()
	p1 := lowTestPort
	p2 := lowTestPort + 1
	ports1 := map[string]uint16{"p1": p1}
	AssertNoError(t, pdb.claimPorts(ports1))

	ports2 := map[string]uint16{"p1": p1, "p2": p2}
	if err := pdb.claimPorts(ports2); err == nil {
		t.Error("intersecting ports should have failed")
	}

	expectedClaimed := map[uint16]struct{}{p1: {}}
	if !reflect.DeepEqual(pdb.ports, expectedClaimed) {
		t.Error("wrong port allocations received:", pdb.ports, expectedClaimed)
	}
}

func TestNextPort(t *testing.T) {
	pdb := newPortDB(7, 9)
	defer pdb.exit()
	var a string
	for i := 0; i < 4; i++ {
		a += fmt.Sprintf("%d", pdb.dynamicRange.nextPort())
	}
	if expected, got := "7897", a; got != expected {
		t.Errorf("expected %s but got %s", expected, got)
	}
}

func AssertNoError(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
}
