package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/bernerdschaefer/eventsource"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestContainerList(t *testing.T) {
	agentTotalMem = 1000
	agentTotalCPU = 2
	configuredVolumes = map[string]struct{}{"/tmp": struct{}{}}

	var (
		registry = newRegistry()
		api      = newAPI(registry)
		server   = httptest.NewServer(api)
	)

	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL+"/api/v0/containers", nil)
	es := eventsource.New(req, -1)
	defer es.Close()

	ev, err := es.Read()
	if err != nil {
		t.Fatal(err)
	}

	var state agent.StateEvent

	if err := json.Unmarshal(ev.Data, &state); err != nil {
		t.Fatal("unable to load containers json:", err)
	}

	expected := agent.HostResources{
		CPUs:    agent.TotalReserved{Total: 2.0, Reserved: 0.0},
		Memory:  agent.TotalReserved{Total: 1000.0, Reserved: 0.0},
		Volumes: []string{"/tmp"},
	}

	if err := validateEvent(expected, state, 0, ""); err != nil {
		t.Error(err)
	}

	cont := newContainer(
		"123",
		agent.ContainerConfig{
			Resources: agent.Resources{
				Memory: 100,
				CPUs:   2,
			},
		},
	)

	registry.m["123"] = cont
	registry.statec <- cont.ContainerInstance

	ev, err = es.Read()
	if err != nil {
		t.Fatal(err)
	}

	if err := json.Unmarshal(ev.Data, &state); err != nil {
		t.Fatal("unable to load containers json:", err)
	}

	expected.CPUs.Reserved = 2
	expected.Memory.Reserved = 100
	if err := validateEvent(expected, state, 1, agent.ContainerStatusCreated); err != nil {
		t.Error(err)
	}

	delete(registry.m, "123")
	cont.ContainerStatus = agent.ContainerStatusDeleted
	registry.statec <- cont.ContainerInstance

	ev, err = es.Read()
	if err != nil {
		t.Fatal(err)
	}

	if err := json.Unmarshal(ev.Data, &state); err != nil {
		t.Fatal("unable to load containers json:", err)
	}

	expected.CPUs.Reserved = 0
	expected.Memory.Reserved = 0
	if err := validateEvent(expected, state, 1, agent.ContainerStatusDeleted); err != nil {
		t.Error(err)
	}
}

func TestLogAPICanTailLogs(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		registry = newRegistry()
		api      = newAPI(registry)
		server   = httptest.NewServer(api)
	)
	defer server.Close()

	createReceiveLogsFixture(t, registry)

	c := newFakeContainer("123")
	registry.register(c)

	// UDP has some weirdness with processing, so we use the container log's subscription
	// mechanism to ensure that we don't run the test until all the messages have been
	// processed.
	linec := make(chan string, 10) // Plenty of room before anything gets dropped
	c.Logs().notify(linec)

	// Send a log line that wil be lost
	sendLog("container[123] m1")
	waitForLogLine(t, linec, time.Second)

	req, err := http.NewRequest("GET", server.URL+"/api/v0/containers/123/log", nil)
	if err != nil {
		t.Fatalf("unable to get log history: %s", err)
	}
	es := eventsource.New(req, time.Second)
	defer es.Close()

	// A tailing reader only receives events sent after it connects, so the test must
	// connect before the test sends the log messages.  Therefore the reader must run
	// concurrently.
	readStepc := make(chan struct{})
	readResultc := make(chan []string)

	// This function performs a read each time it receives a write on the channel readStepc.
	// It writes the results to readResultc.  The goroutine terminates when readStepc
	// closes.  Any error causes test failure.
	go func() {
		for _ = range readStepc {
			ev, err := es.Read()
			if err != nil {
				t.Fatal(err)
			}
			var logLines []string
			if err := json.Unmarshal(ev.Data, &logLines); err != nil {
				t.Fatalf("unable to load containers json: %s", err)
			}
			readResultc <- logLines
		}
	}()
	defer close(readStepc)

	readStepc <- struct{}{} // Initial read causes a connection.
	// Horrible, horrible hack to get ensure that the eventstream.Read() has time to connect.
	time.Sleep(time.Second)

	sendLog("container[123] m2")
	sendLog("container[123] m3")

	logLines := <-readResultc
	ExpectArraysEqual(t, logLines, []string{"container[123] m2"})

	readStepc <- struct{}{}
	logLines = <-readResultc
	ExpectArraysEqual(t, logLines, []string{"container[123] m3"})
}

func TestLogAPICanRetrieveLastLines(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		registry = newRegistry()
		api      = newAPI(registry)
		server   = httptest.NewServer(api)
	)
	defer server.Close()

	createReceiveLogsFixture(t, registry)

	c := newFakeContainer("123")
	registry.register(c)

	// UDP has some weirdness with processing, so we use the container log's subscription
	// mechanism to ensure that we don't run the test until all the messages have been
	// processed.
	linec := make(chan string, 10) // Plenty of room before anything gets dropped
	c.Logs().notify(linec)

	// Send two log messages out, wait for their reception, and then check for them in the
	// log history.
	sendLog("container[123] m1")
	sendLog("container[123] m2")

	waitForLogLine(t, linec, time.Second)
	waitForLogLine(t, linec, time.Second)

	resp, err := http.Get(server.URL + "/api/v0/containers/123/log?history=3")
	if err != nil {
		t.Fatalf("unable to get log history: %s", err)
	}
	logLines := []string{}
	if err = json.NewDecoder(resp.Body).Decode(&logLines); err != nil {
		t.Fatalf("unable to read json response: %s", err)
	}

	ExpectArraysEqual(t, logLines, []string{"container[123] m1", "container[123] m2"})
}

func TestMessagesGetWrittenToLogs(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	registry := newRegistry()

	createReceiveLogsFixture(t, registry)

	c := newFakeContainer("123")
	registry.register(c)

	// UDP has some weirdness with processing, so we use the container log's subscription
	// mechanism to ensure that we don't run the test until all the messages have been
	// processed.
	linec := make(chan string, 10) // Plenty of room before anything gets dropped
	c.Logs().notify(linec)

	// Send two log messages out
	sendLog("container[123] m1")
	sendLog("container[123] m2")

	// Wait for both messages to come in.
	waitForLogLine(t, linec, time.Second)
	waitForLogLine(t, linec, time.Second)

	logLines := c.Logs().last(3)
	ExpectArraysEqual(t, logLines, []string{"container[123] m1", "container[123] m2"})
}

func TestLogRoutingOfDefectiveMessages(t *testing.T) {
	registry := newRegistry()

	createReceiveLogsFixture(t, registry)

	c := newFakeContainer("123")
	registry.register(c)

	linec := make(chan string, 10) // Plenty of room before anything gets dropped
	c.Logs().notify(linec)

	sendLog("ilj;irtr") // Should not be received

	// TODO(jmy): In the future make sure that "unroutable message" counter goes up.
	// For now a timeout is a workable substitute for verifying that a log message was
	// not routed.  It's better than nothing.
	expectNoLogLines(t, linec, 100*time.Millisecond)
}

func waitForLogLine(t *testing.T, c chan string, timeout time.Duration) {
	select {
	case <-c:
	case <-time.After(timeout):
		t.Errorf("Did not receive an item within %s", timeout)
	}
}

func expectNoLogLines(t *testing.T, c chan string, timeout time.Duration) {
	select {
	case logLine := <-c:
		t.Errorf("Nothing should have been received, but got: %s", logLine)
	case <-time.After(timeout):
	}
}

func sendLog(logLine string) error {
	conn, _ := net.Dial("udp", *logAddr)
	buf := []byte(logLine)
	_, err := conn.Write(buf)
	return err
}

func setLogAddrRandomly(t *testing.T) {
	port, err := GetRandomUDPPort()
	if err != nil {
		t.Fatalf("Could not locate a random port: %s", err)
	}
	*logAddr = "localhost:" + strconv.Itoa(port)
}

// GetRandomUDPPort identifies a UDP port by attempting to connect to port zero. This
// port may-or-may-not-be available when you attempt to use it, but it's better than
// nothing.
func GetRandomUDPPort() (int, error) {
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

func validateEvent(expected agent.HostResources, have agent.StateEvent, containersCount int, status agent.ContainerStatus) error {
	if expected.CPUs != have.Resources.CPUs {
		return fmt.Errorf("invalid cpu resources: expected %v != have %v", expected.CPUs, have.Resources.CPUs)
	}

	if expected.Memory != have.Resources.Memory {
		return fmt.Errorf("invalid memory resources: expected %v != have %v", expected.Memory, have.Resources.Memory)
	}

	if !reflect.DeepEqual(expected.Volumes, have.Resources.Volumes) {
		return fmt.Errorf("invalid volumes : expected %v != have %v", expected.Volumes, have.Resources.Volumes)
	}

	if len(have.Containers) != containersCount {
		return fmt.Errorf("invalid number of containers in delta update, expected %d != %d", len(have.Containers), containersCount)
	}

	if containersCount == 0 {
		return nil
	}

	if container, ok := have.Containers["123"]; !ok || container.ContainerStatus != status {
		return fmt.Errorf("container event invalid")
	}

	return nil
}
