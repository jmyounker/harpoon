package main

import (
	"bytes"
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

// If we ever start getting collisions during testing then we can point this to a
// temp directory.
const fixtureContainerRoot = "/run/harpoon"

func TestContainerList(t *testing.T) {
	agentMem = 1000
	agentCPU = 2
	configuredVolumes = map[string]struct{}{"/tmp": struct{}{}}

	var (
		registry = newRegistry()
		pdb      = newPortDB(lowTestPort, highTestPort)
		api      = newAPI(fixtureContainerRoot, registry, pdb)
		server   = httptest.NewServer(api)
	)

	defer pdb.exit()
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
		CPU:     agent.TotalReserved{Total: 2.0, Reserved: 0.0},
		Mem:     agent.TotalReservedInt{Total: 1000.0, Reserved: 0.0},
		Volumes: []string{"/tmp"},
	}

	if err := validateEvent(expected, state, 0, ""); err != nil {
		t.Error(err)
	}

	cont := newFakeContainer(
		"123",
		"",
		agent.ContainerConfig{
			Resources: agent.Resources{
				Mem: 100,
				CPU: 2,
			},
		},
		nil)

	registry.m["123"] = cont
	registry.statec <- cont.Instance()

	ev, err = es.Read()
	if err != nil {
		t.Fatal(err)
	}

	if err := json.Unmarshal(ev.Data, &state); err != nil {
		t.Fatal("unable to load containers json:", err)
	}

	expected.CPU.Reserved = 2
	expected.Mem.Reserved = 100
	if err := validateEvent(expected, state, 1, agent.ContainerStatusRunning); err != nil {
		t.Error(err)
	}

	cont.Destroy()
	cont.Exit()

	registry.statec <- cont.Instance()

	ev, err = es.Read()
	if err != nil {
		t.Fatal(err)
	}

	if err := json.Unmarshal(ev.Data, &state); err != nil {
		t.Fatal("unable to load containers json:", err)
	}

	expected.CPU.Reserved = 0
	expected.Mem.Reserved = 0
	if err := validateEvent(expected, state, 1, agent.ContainerStatusDeleted); err != nil {
		t.Error(err)
	}
}

func TestResources(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	agentMem = 1000
	agentCPU = 2
	configuredVolumes = map[string]struct{}{"/tmp": struct{}{}}
	newContainer = newFakeContainer

	var (
		registry = newRegistry()
		pdb      = newPortDB(lowTestPort, highTestPort)
		api      = newAPI(fixtureContainerRoot, registry, pdb)
		server   = httptest.NewServer(api)
	)
	defer pdb.exit()
	defer server.Close()

	have, err := getResources(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	expected := agent.HostResources{
		CPU:     agent.TotalReserved{Total: 2.0, Reserved: 0.0},
		Mem:     agent.TotalReservedInt{Total: 1000.0, Reserved: 0.0},
		Volumes: []string{"/tmp"},
	}
	if err := validateResources(expected, have); err != nil {
		t.Fatal(err)
	}

	config := agent.ContainerConfig{
		Resources: agent.Resources{
			Mem: 100,
			CPU: 2,
		},
	}

	if err := createContainer(server.URL, config, "containerID"); err != nil {
		t.Fatal(err)
	}

	have, err = getResources(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	expected = agent.HostResources{
		CPU:     agent.TotalReserved{Total: 2.0, Reserved: 2.0},
		Mem:     agent.TotalReservedInt{Total: 1000.0, Reserved: 100.0},
		Volumes: []string{"/tmp"},
	}
	if err := validateResources(expected, have); err != nil {
		t.Fatal(err)
	}

	if err := deleteContainer(server.URL, "containerID"); err != nil {
		t.Fatal(err)
	}

	have, err = getResources(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	expected = agent.HostResources{
		CPU:     agent.TotalReserved{Total: 2.0, Reserved: 0.0},
		Mem:     agent.TotalReservedInt{Total: 1000.0, Reserved: 0.0},
		Volumes: []string{"/tmp"},
	}
	if err := validateResources(expected, have); err != nil {
		t.Fatal(err)
	}
}

func TestLogAPICanTailLogs(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		registry = newRegistry()
		pdb      = newPortDB(lowTestPort, highTestPort)
		api      = newAPI(fixtureContainerRoot, registry, pdb)
		server   = httptest.NewServer(api)
	)
	defer pdb.exit()
	defer server.Close()

	createReceiveLogsFixture(t, registry)

	c := newFakeContainer("123", "", agent.ContainerConfig{}, nil)
	registry.register(c)

	// UDP has some weirdness with processing, so we use the container log's subscription
	// mechanism to ensure that we don't run the test until all the messages have been
	// processed.
	linec := make(chan string, 10) // Plenty of room before anything gets dropped
	c.Logs().notify(linec)

	// Send a log line that wil be lost
	sendLog("container[123] m1")
	waitForLogLine(t, linec, time.Second)

	// history=0 forces logging to ignore all previous history
	req, err := http.NewRequest("GET", server.URL+"/api/v0/containers/123/log?history=0", nil)
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

	// Horrible, horrible hack to ensure that the eventstream.Read() has time to connect.
	time.Sleep(time.Second)

	sendLog("container[123] m2")
	sendLog("container[123] m3")

	logLines := <-readResultc
	ExpectArraysEqual(t, logLines, []string{"container[123] m2"})

	readStepc <- struct{}{}
	logLines = <-readResultc
	ExpectArraysEqual(t, logLines, []string{"container[123] m3"})
}

func TestLogAPILogTailIncludesHistory(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		registry = newRegistry()
		pdb      = newPortDB(lowTestPort, highTestPort)
		api      = newAPI(fixtureContainerRoot, registry, pdb)
		server   = httptest.NewServer(api)
	)
	defer pdb.exit()
	defer server.Close()

	createReceiveLogsFixture(t, registry)

	c := newFakeContainer("123", "", agent.ContainerConfig{}, nil)
	registry.register(c)

	// UDP has some weirdness with processing, so we use the container log's subscription
	// mechanism to ensure that we don't run the test until all the messages have been
	// processed.
	linec := make(chan string, 10) // Plenty of room before anything gets dropped
	c.Logs().notify(linec)

	// Send a log line that will be lost
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
	// Horrible, horrible hack to ensure that the eventstream.Read() has time to connect.
	time.Sleep(time.Second)

	sendLog("container[123] m2")
	sendLog("container[123] m3")

	logLines := <-readResultc
	ExpectArraysEqual(t, logLines, []string{"container[123] m1"})

	readStepc <- struct{}{}
	logLines = <-readResultc
	ExpectArraysEqual(t, logLines, []string{"container[123] m2"})

	readStepc <- struct{}{}
	logLines = <-readResultc
	ExpectArraysEqual(t, logLines, []string{"container[123] m3"})
}

func TestLogAPICanRetrieveLastLines(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		registry = newRegistry()
		pdb      = newPortDB(lowTestPort, highTestPort)
		api      = newAPI(fixtureContainerRoot, registry, pdb)
		server   = httptest.NewServer(api)
	)
	defer pdb.exit()
	defer server.Close()

	createReceiveLogsFixture(t, registry)

	c := newFakeContainer("123", "", agent.ContainerConfig{}, nil)
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

	c := newFakeContainer("123", "", agent.ContainerConfig{}, nil)
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

	c := newFakeContainer("123", "", agent.ContainerConfig{}, nil)
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
	conn, _ := net.Dial("udp", logAddr)
	buf := []byte(logLine)
	_, err := conn.Write(buf)
	return err
}

func setLogAddrRandomly(t *testing.T) {
	port, err := getRandomUDPPort()
	if err != nil {
		t.Fatalf("could not locate a random port: %s", err)
	}
	logAddr = "localhost:" + strconv.Itoa(port)
}

func validateEvent(expected agent.HostResources, have agent.StateEvent, containersCount int, status agent.ContainerStatus) error {
	if err := validateResources(expected, have.Resources); err != nil {
		return err
	}

	if len(have.Containers) != containersCount {
		return fmt.Errorf("invalid number of containers in delta update, expected %d != %d", len(have.Containers), containersCount)
	}

	if containersCount == 0 {
		return nil
	}

	container, ok := have.Containers["123"]
	if !ok {
		return fmt.Errorf("container event not received")
	}

	if container.ContainerStatus != status {
		return fmt.Errorf("invalid status, expected: %s != %s", status, container.ContainerStatus)
	}

	return nil
}

func getResources(url string) (agent.HostResources, error) {
	resp, err := http.Get(url + "/api/v0/resources")
	if err != nil {
		return agent.HostResources{}, fmt.Errorf("unable to get resources: %s", err)
	}
	defer resp.Body.Close()

	var resources agent.HostResources
	if err = json.NewDecoder(resp.Body).Decode(&resources); err != nil {
		return agent.HostResources{}, fmt.Errorf("unable to read json response: %s", err)
	}

	return resources, nil
}

func validateResources(expected agent.HostResources, have agent.HostResources) error {
	if expected.CPU != have.CPU {
		return fmt.Errorf("invalid cpu resources: expected %v != have %v", expected.CPU, have.CPU)
	}

	if expected.Mem != have.Mem {
		return fmt.Errorf("invalid memory resources: expected %v != have %v", expected.Mem, have.Mem)
	}

	if !reflect.DeepEqual(expected.Volumes, have.Volumes) {
		return fmt.Errorf("invalid volumes : expected %v != have %v", expected.Volumes, have.Volumes)
	}

	return nil
}

func createContainer(url string, config agent.ContainerConfig, id string) error {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(config); err != nil {
		return fmt.Errorf("problem encoding container config (%s)", err)
	}

	req, err := http.NewRequest("PUT", url+"/api/v0/containers/"+id, &body)
	if err != nil {
		return fmt.Errorf("problem constructing HTTP request (%s)", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not create container")
	}
	defer resp.Body.Close()

	return nil
}

func deleteContainer(url string, id string) error {
	req, err := http.NewRequest("DELETE", url+"/api/v0/containers/"+id, nil)
	if err != nil {
		return fmt.Errorf("problem constructing HTTP request (%s)", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not delete container")
	}
	defer resp.Body.Close()

	return nil
}
