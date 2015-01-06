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

	if err != nil {
		t.Fatal(err)
	}
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
	want := map[string]uint16{"p1": p1}

	err := pdb.acquirePorts(ports)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(ports, want) {
		t.Error("wrong port allocations received:", want)
	}
}

func TestGetPortRecordsAllocation(t *testing.T) {
	pdb := newPortDB(lowTestPort, highTestPort)
	defer pdb.exit()
	p1 := lowTestPort - 2
	ports := map[string]uint16{"p1": p1}

	err := pdb.acquirePorts(ports)
	if err != nil {
		t.Fatal(err)
	}

	allocations := pdb.allocations()
	if _, ok := allocations[p1]; !ok {
		t.Error("could not claim port")
	}
}

func TestReleasePortRemovesAllocation(t *testing.T) {
	pdb := newPortDB(lowTestPort, highTestPort)
	defer pdb.exit()
	p1 := lowTestPort - 2
	ports := map[string]uint16{"p1": p1}

	err := pdb.acquirePorts(ports)
	if err != nil {
		t.Fatal(err)
	}

	pdb.releasePorts(ports)
	allocations := pdb.allocations()
	if _, ok := allocations[p1]; ok {
		t.Error("could not claim port")
	}
}

func TestCantGetAPortFromAFullyAllocatedRange(t *testing.T) {
	pdb := newPortDB(lowTestPort, lowTestPort+2)
	defer pdb.exit()
	ports := map[string]uint16{"p1": 0, "p2": 0, "p3": 0, "p4": 0}

	if err := pdb.acquirePorts(ports); err == nil {
		t.Error("want error")
	}
}

func TestDynamicAllocationFailureDoesNotAlterRequestedPorts(t *testing.T) {
	pdb := newPortDB(lowTestPort, lowTestPort+1)
	defer pdb.exit()
	p4 := lowTestPort - 2
	ports := map[string]uint16{"p1": 0, "p2": 0, "p3": 0, "p4": p4}

	pdb.acquirePorts(ports)

	wantPorts := map[string]uint16{"p1": 0, "p2": 0, "p3": 0, "p4": p4}
	if !reflect.DeepEqual(ports, wantPorts) {
		t.Error("want no alterations but received", ports, "when expecting", wantPorts)
	}
}

func TestCanGetPortFromOneElementRange(t *testing.T) {
	pdb := newPortDB(lowTestPort, lowTestPort)
	defer pdb.exit()
	ports := map[string]uint16{"p1": 0}

	err := pdb.acquirePorts(ports)
	if err != nil {
		t.Error("want error")
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
		t.Error("want error")
	}
}

func TestClaimPort(t *testing.T) {
	pdb := newPortDB(lowTestPort, lowTestPort)
	defer pdb.exit()

	p1 := lowTestPort - 2
	ports := map[string]uint16{"p1": p1}

	pdb.claimPorts(ports)

	allocations := pdb.allocations()
	if _, ok := allocations[p1]; !ok {
		t.Error("could not claim port")
	}
}

func TestStaticReacquisitionFailsWithoutAlteringClaimedPorts(t *testing.T) {
	pdb := newPortDB(lowTestPort, lowTestPort)
	defer pdb.exit()
	p1 := lowTestPort - 2
	p2 := lowTestPort - 1
	ports1 := map[string]uint16{"p1": p1}

	if err := pdb.claimPorts(ports1); err != nil {
		t.Fatal(err)
	}

	ports2 := map[string]uint16{"p1": p1, "p2": p2}
	if err := pdb.acquirePorts(ports2); err == nil {
		t.Error("intersecting ports should have failed")
	}

	if want, have := map[uint16]struct{}{p1: {}}, pdb.allocations(); !reflect.DeepEqual(want, have) {
		t.Errorf("wrong port allocations received: want %v, have %v", want, have)
	}
}

func TestDynamicReacquisitionFailsWithoutAlteringClaimedPorts(t *testing.T) {
	pdb := newPortDB(lowTestPort, lowTestPort)
	defer pdb.exit()

	ports1 := map[string]uint16{"p1": 0}

	if err := pdb.acquirePorts(ports1); err != nil {
		t.Fatal(err)
	}

	p1 := ports1["p1"]
	ports2 := map[string]uint16{"p1": p1, "p2": 0}

	if err := pdb.acquirePorts(ports2); err == nil {
		t.Error("intersecting ports should have failed")
	}

	if want, have := map[uint16]struct{}{p1: struct{}{}}, pdb.allocations(); !reflect.DeepEqual(want, have) {
		t.Errorf("wrong port allocations received: want %v, have %v", want, have)
	}
}

func TestReclaimingFailsWithoutAlteringClaimedPorts(t *testing.T) {
	pdb := newPortDB(lowTestPort, lowTestPort)
	defer pdb.exit()
	p1 := lowTestPort
	p2 := lowTestPort + 1
	ports1 := map[string]uint16{"p1": p1}

	if err := pdb.claimPorts(ports1); err != nil {
		t.Fatal(err)
	}

	ports2 := map[string]uint16{"p1": p1, "p2": p2}
	if err := pdb.claimPorts(ports2); err == nil {
		t.Error("intersecting ports should have failed")
	}

	if want, have := map[uint16]struct{}{p1: {}}, pdb.allocations(); !reflect.DeepEqual(want, have) {
		t.Errorf("wrong port allocations received: want %v, have %v", want, have)
	}
}

func TestNextPort(t *testing.T) {
	dr := newPortRange(7, 9)

	var a string
	for i := 0; i < 4; i++ {
		a += fmt.Sprintf("%d", dr.nextPort())
	}

	if want, have := "7897", a; want != have {
		t.Errorf("want %s, have %s", want, have)
	}
}
