package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"testing"
	"time"
)

var (
	// Chosen so they don't collide with the production range.
	lowTestPort  = uint16(rand.Intn((23000-100)-20000) + 20000)
	highTestPort = lowTestPort + 100
)

// getRandomUDPPort identifies a UDP port by attempting to connect to port zero. This
// port may-or-may-not-be available when you attempt to use it, but it's better than
// nothing.
func getRandomUDPPort() (int, error) {
	laddr, err := net.ResolveUDPAddr("udp", ":0")
	if err != nil {
		return 0, err
	}

	ln, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.LocalAddr().(*net.UDPAddr).Port, nil
}

func dumpJSONPretty(v interface{}) string {
	x, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("CONVERSTION ERROR: %s", err)
	}
	return string(x)
}

func createReceiveLogsFixture(t *testing.T, r *registry) {
	setLogAddrRandomly(t)
	go receiveLogs(r)
}

func setLogAddrRandomly(t *testing.T) {
	port, err := getRandomUDPPort()
	if err != nil {
		t.Fatalf("could not locate a random port: %s", err)
	}
	logAddr = "localhost:" + strconv.Itoa(port)
}

func waitForLogLine(t *testing.T, c chan string, timeout time.Duration) {
	select {
	case <-c:
	case <-time.After(timeout):
		t.Errorf("did not receive an item within %s", timeout)
	}
}

func expectNoLogLines(t *testing.T, c chan string, timeout time.Duration) {
	select {
	case logLine := <-c:
		t.Errorf("nothing should have been received, but got: %s", logLine)
	case <-time.After(timeout):
	}
}

func sendLog(logLine string) error {
	conn, err := net.Dial("udp", logAddr)
	if err != nil {
		return err
	}

	_, err = conn.Write([]byte(logLine))
	return err
}
