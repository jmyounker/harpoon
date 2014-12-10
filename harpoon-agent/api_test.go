package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/bernerdschaefer/eventsource"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestContainerList(t *testing.T) {
	testContainerRoot, err := ioutil.TempDir(os.TempDir(), "harpoon-agent-api-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testContainerRoot)

	var (
		agentMem          int64   = 1000
		agentCPU          float64 = 2
		configuredVolumes         = map[string]struct{}{"/tmp": struct{}{}}
		debug                     = false
		timeout                   = agent.DefaultDownloadTimeout

		registry = newRegistry(nopServiceDiscovery{})
		pdb      = newPortDB(lowTestPort, highTestPort)
		api      = newAPI(testContainerRoot, registry, pdb, configuredVolumes, agentCPU, agentMem, timeout, debug)
		server   = httptest.NewServer(api)
	)

	defer pdb.exit()
	defer server.Close()

	req, err := http.NewRequest("GET", server.URL+agent.APIVersionPrefix+agent.APIListContainersPath, nil)
	if err != nil {
		t.Fatal(err)
	}

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

	want := agent.HostResources{
		CPU:     agent.TotalReserved{Total: 2.0, Reserved: 0.0},
		Mem:     agent.TotalReservedInt{Total: 1000.0, Reserved: 0.0},
		Volumes: []string{"/tmp"},
	}

	if err := validateEvent(want, state, 0, ""); err != nil {
		t.Error(err)
	}

	cont := newFakeContainer(
		"123",
		"",
		volumes{},
		agent.ContainerConfig{
			Resources: agent.Resources{
				Mem: 100,
				CPU: 2,
			},
		},
		false,
		nil,
	)

	registry.register(cont)
	registry.statec <- cont.Instance()

	ev, err = es.Read()
	if err != nil {
		t.Fatal(err)
	}

	if err := json.Unmarshal(ev.Data, &state); err != nil {
		t.Fatal("unable to load containers json:", err)
	}

	want.CPU.Reserved = 2
	want.Mem.Reserved = 100
	if err := validateEvent(want, state, 1, agent.ContainerStatusRunning); err != nil {
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

	want.CPU.Reserved = 0
	want.Mem.Reserved = 0
	if err := validateEvent(want, state, 1, agent.ContainerStatusDeleted); err != nil {
		t.Error(err)
	}
}

func TestAgentHostResources(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	// If we don't do this, newRealContainer will try to mutate the
	// /srv/harpoon filesystem, which doesn't necessarily exist.
	newContainer = newFakeContainer

	// Set up an empty agent
	testContainerRoot, err := ioutil.TempDir(os.TempDir(), "harpoon-agent-api-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testContainerRoot)

	var (
		agentMem          int64   = 1000
		agentCPU          float64 = 2
		configuredVolumes         = map[string]struct{}{"/tmp": struct{}{}}
		debug                     = false
		timeout                   = agent.DefaultDownloadTimeout

		registry  = newRegistry(nopServiceDiscovery{})
		pdb       = newPortDB(lowTestPort, highTestPort)
		api       = newAPI(testContainerRoot, registry, pdb, configuredVolumes, agentCPU, agentMem, timeout, debug)
		server    = httptest.NewServer(api)
		client, _ = agent.NewClient(server.URL)
	)
	defer pdb.exit()
	defer server.Close()

	// Verify the HostResources of the empty agent

	have, err := client.Resources()
	if err != nil {
		t.Fatal(err)
	}

	want := agent.HostResources{
		CPU:     agent.TotalReserved{Total: 2.0, Reserved: 0.0},
		Mem:     agent.TotalReservedInt{Total: 1000.0, Reserved: 0.0},
		Volumes: []string{"/tmp"},
	}
	if err := validateResources(want, have); err != nil {
		t.Fatal(err)
	}

	// Create a container with some resource reservations

	var (
		containerID = "123"
		config      = agent.ContainerConfig{
			Resources: agent.Resources{
				Mem: 100,
				CPU: 2,
			},
		}
	)

	if err := client.Put(containerID, config); err != nil {
		t.Fatal(err)
	}

	// Verify the HostResources are as we expect

	have, err = client.Resources()
	if err != nil {
		t.Fatal(err)
	}

	want = agent.HostResources{
		CPU:     agent.TotalReserved{Total: 2.0, Reserved: 2.0},
		Mem:     agent.TotalReservedInt{Total: 1000.0, Reserved: 100.0},
		Volumes: []string{"/tmp"},
	}
	if err := validateResources(want, have); err != nil {
		t.Fatal(err)
	}

	// Destroy the container

	if err := client.Destroy(containerID); err != nil {
		t.Fatal(err)
	}

	// Verify the HostResources are reclaimed

	have, err = client.Resources()
	if err != nil {
		t.Fatal(err)
	}

	want = agent.HostResources{
		CPU:     agent.TotalReserved{Total: 2.0, Reserved: 0.0},
		Mem:     agent.TotalReservedInt{Total: 1000.0, Reserved: 0.0},
		Volumes: []string{"/tmp"},
	}
	if err := validateResources(want, have); err != nil {
		t.Fatal(err)
	}
}

func TestLogAPICanTailLogs(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	testContainerRoot, err := ioutil.TempDir(os.TempDir(), "harpoon-agent-api-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testContainerRoot)

	var (
		agentMem          int64
		agentCPU          float64
		configuredVolumes map[string]struct{}
		debug             = false
		timeout           = agent.DefaultDownloadTimeout

		registry = newRegistry(nopServiceDiscovery{})
		pdb      = newPortDB(lowTestPort, highTestPort)
		api      = newAPI(testContainerRoot, registry, pdb, configuredVolumes, agentCPU, agentMem, timeout, debug)
		server   = httptest.NewServer(api)
	)
	defer pdb.exit()
	defer server.Close()

	createReceiveLogsFixture(t, registry)

	c := newFakeContainer("123", "", volumes{}, agent.ContainerConfig{}, false, nil)
	registry.register(c)

	// UDP has some weirdness with processing, so we use the container log's subscription
	// mechanism to ensure that we don't run the test until all the messages have been
	// processed.
	linec := make(chan string, 10) // Plenty of room before anything gets dropped
	c.Logs().notify(linec)

	// Send a log line that will be lost
	sendLog("container[123] m1")
	waitForLogLine(t, linec, time.Second)

	// history=0 forces logging to ignore all previous history
	req, err := http.NewRequest("GET", server.URL+agent.APIVersionPrefix+"/containers/123/log?history=0", nil)
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
	expectArraysEqual(t, logLines, []string{"container[123] m2"})

	readStepc <- struct{}{}
	logLines = <-readResultc
	expectArraysEqual(t, logLines, []string{"container[123] m3"})
}

func TestLogAPILogTailIncludesHistory(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	testContainerRoot, err := ioutil.TempDir(os.TempDir(), "harpoon-agent-api-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testContainerRoot)

	var (
		agentMem          int64   = 1000
		agentCPU          float64 = 2
		configuredVolumes         = map[string]struct{}{"/tmp": struct{}{}}
		debug                     = false
		timeout                   = agent.DefaultDownloadTimeout

		registry = newRegistry(nopServiceDiscovery{})
		pdb      = newPortDB(lowTestPort, highTestPort)
		api      = newAPI(testContainerRoot, registry, pdb, configuredVolumes, agentCPU, agentMem, timeout, debug)
		server   = httptest.NewServer(api)
	)
	defer pdb.exit()
	defer server.Close()

	createReceiveLogsFixture(t, registry)

	c := newFakeContainer("123", "", volumes{}, agent.ContainerConfig{}, false, nil)
	registry.register(c)

	// UDP has some weirdness with processing, so we use the container log's subscription
	// mechanism to ensure that we don't run the test until all the messages have been
	// processed.
	linec := make(chan string, 10) // Plenty of room before anything gets dropped
	c.Logs().notify(linec)

	// Send a log line that will be lost
	sendLog("container[123] m1")
	waitForLogLine(t, linec, time.Second)

	req, err := http.NewRequest("GET", server.URL+agent.APIVersionPrefix+"/containers/123/log", nil)
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
	expectArraysEqual(t, logLines, []string{"container[123] m1"})

	readStepc <- struct{}{}
	logLines = <-readResultc
	expectArraysEqual(t, logLines, []string{"container[123] m2"})

	readStepc <- struct{}{}
	logLines = <-readResultc
	expectArraysEqual(t, logLines, []string{"container[123] m3"})
}

func TestLogAPICanRetrieveLastLines(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	testContainerRoot, err := ioutil.TempDir(os.TempDir(), "harpoon-agent-api-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testContainerRoot)

	var (
		agentMem          int64   = 1000
		agentCPU          float64 = 2
		configuredVolumes         = map[string]struct{}{"/tmp": struct{}{}}
		debug                     = false
		timeout                   = agent.DefaultDownloadTimeout

		registry = newRegistry(nopServiceDiscovery{})
		pdb      = newPortDB(lowTestPort, highTestPort)
		api      = newAPI(testContainerRoot, registry, pdb, configuredVolumes, agentCPU, agentMem, timeout, debug)
		server   = httptest.NewServer(api)
	)
	defer pdb.exit()
	defer server.Close()

	createReceiveLogsFixture(t, registry)

	c := newFakeContainer("123", "", volumes{}, agent.ContainerConfig{}, false, nil)
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

	resp, err := http.Get(server.URL + agent.APIVersionPrefix + "/containers/123/log?history=3")
	if err != nil {
		t.Fatalf("unable to get log history: %s", err)
	}
	logLines := []string{}
	if err = json.NewDecoder(resp.Body).Decode(&logLines); err != nil {
		t.Fatalf("unable to read json response: %s", err)
	}

	expectArraysEqual(t, logLines, []string{"container[123] m1", "container[123] m2"})
}

func TestFailedCreateDestroysContainer(t *testing.T) {
	// There are many ways to fail:
	//
	// - a.registry.register fails (already exists)
	// - container.Create fails (invalid config; can't assign ports; mkdir run/logdir fails; agent.json fails; fetch fails; symlink fails)
	// - container.Start fails (bad initial status; supervisor log create fails; startLogger fails; supervisor create fails)
	//
	// This test just captures one of them: container.Create fails because fetch fails.

	testContainerRoot, err := ioutil.TempDir(os.TempDir(), "harpoon-agent-api-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testContainerRoot)

	// If we don't do this, newRealContainer will try to mutate the
	// /srv/harpoon filesystem, which doesn't necessarily exist.
	newContainer = newFakeContainer

	var (
		agentMem          int64   = 1000
		agentCPU          float64 = 2
		configuredVolumes         = map[string]struct{}{"/tmp": struct{}{}}
		debug                     = false
		timeout                   = agent.DefaultDownloadTimeout

		registry = newRegistry(nopServiceDiscovery{})
		pdb      = newPortDB(lowTestPort, highTestPort)
		api      = newAPI(testContainerRoot, registry, pdb, configuredVolumes, agentCPU, agentMem, timeout, debug)
		server   = httptest.NewServer(api)
		client   = agent.MustNewClient(server.URL)
	)
	defer pdb.exit()
	defer server.Close()

	err = client.Put("foo", agent.ContainerConfig{ArtifactURL: failingArtifactURL})
	if err == nil {
		t.Fatalf("expected error, got none")
	}

	t.Logf("got expected error (%v)", err)

	_, err = client.Get("foo")
	if err == nil {
		t.Fatal("expected error, got none")
	}

	t.Logf("got expected error (%v)", err)
}

func validateEvent(want agent.HostResources, have agent.StateEvent, containersCount int, status agent.ContainerStatus) error {
	if err := validateResources(want, have.Resources); err != nil {
		return err
	}

	if containersCount != len(have.Containers) {
		return fmt.Errorf("invalid number of containers in delta update, want %d, have %d", containersCount, len(have.Containers))
	}

	if containersCount == 0 {
		return nil
	}

	container, ok := have.Containers["123"]
	if !ok {
		return fmt.Errorf("container event not received")
	}

	if container.ContainerStatus != status {
		return fmt.Errorf("invalid status, want: %s, have %s", status, container.ContainerStatus)
	}

	return nil
}

func validateResources(want agent.HostResources, have agent.HostResources) error {
	if want.CPU != have.CPU {
		return fmt.Errorf("invalid CPU resources: want %v, have %v", want.CPU, have.CPU)
	}

	if want.Mem != have.Mem {
		return fmt.Errorf("invalid mem resources: want %v, have %v", want.Mem, have.Mem)
	}

	if !reflect.DeepEqual(want.Volumes, have.Volumes) {
		return fmt.Errorf("invalid volumes: want %v, have %v", want.Volumes, have.Volumes)
	}

	return nil
}
