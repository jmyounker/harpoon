package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/bernerdschaefer/eventsource"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestContainerList(t *testing.T) {
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

	var containers []agent.ContainerInstance

	if err := json.Unmarshal(ev.Data, &containers); err != nil {
		t.Fatal("unable to load containers json:", err)
	}

	registry.statec <- agent.ContainerInstance{ID: "123"}

	ev, err = es.Read()
	if err != nil {
		t.Fatal(err)
	}

	if err := json.Unmarshal(ev.Data, &containers); err != nil {
		t.Fatal("unable to load containers json:", err)
	}

	if len(containers) != 1 {
		t.Fatal("invalid number of containers in delta update")
	}

	if containers[0].ID != "123" {
		t.Fatal("container event invalid")
	}
}

func TestLogAPICanTailLogs(t *testing.T) {
	var (
		registry = newRegistry()
		api      = newAPI(registry)
		server   = httptest.NewServer(api)
	)
	defer server.Close()

	setLogPortRandomly(t)
	go receiveLogs(registry)

	c := newFakeContainer("123")
	registry.Register(c)

	// UDP has some weirdness with processing, so we use the container log's subscription
	// mechanism to ensure that we don't run the test until all the messages have been
	// processed.
	logLinec := make(chan string, 10) // Plenty of room before anything gets dropped
	c.Logs().Notify(logLinec)

	// Send a log line that wil be lost
	sendLog("container[123] m1")
	waitForLogLine(t, logLinec, time.Second)

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
	var (
		registry = newRegistry()
		api      = newAPI(registry)
		server   = httptest.NewServer(api)
	)
	defer server.Close()

	setLogPortRandomly(t)
	go receiveLogs(registry)

	c := newFakeContainer("123")
	registry.Register(c)

	// UDP has some weirdness with processing, so we use the container log's subscription
	// mechanism to ensure that we don't run the test until all the messages have been
	// processed.
	logLinec := make(chan string, 10) // Plenty of room before anything gets dropped
	c.Logs().Notify(logLinec)

	// Send two log messages out, wait for their reception, and then check for them in the
	// log history.
	sendLog("container[123] m1")
	sendLog("container[123] m2")

	waitForLogLine(t, logLinec, time.Second)
	waitForLogLine(t, logLinec, time.Second)

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
	registry := newRegistry()

	setLogPortRandomly(t)
	go receiveLogs(registry)

	c := newFakeContainer("123")
	registry.Register(c)

	// UDP has some weirdness with processing, so we use the container log's subscription
	// mechanism to ensure that we don't run the test until all the messages have been
	// processed.
	logLinec := make(chan string, 10) // Plenty of room before anything gets dropped
	c.Logs().Notify(logLinec)

	// Send two log messages out
	sendLog("container[123] m1")
	sendLog("container[123] m2")

	// Wait for both messages to come in.
	waitForLogLine(t, logLinec, time.Second)
	waitForLogLine(t, logLinec, time.Second)

	logLines := c.Logs().Last(3)
	ExpectArraysEqual(t, logLines, []string{"container[123] m1", "container[123] m2"})
}

func TestLogRoutingOfDefectiveMessages(t *testing.T) {
	registry := newRegistry()

	setLogPortRandomly(t)
	go receiveLogs(registry)

	c := newFakeContainer("123")
	registry.Register(c)

	logLinec := make(chan string, 10) // Plenty of room before anything gets dropped
	c.Logs().Notify(logLinec)

	sendLog("ilj;irtr") // Should not be received

	// TODO(jmy): In the future make sure that "unroutable message" counter goes up.
	// For now a timeout is a workable substitute for verifying that a log message was
	// not routed.  It's better than nothing.
	expectNoLogLines(t, logLinec, 100*time.Millisecond)
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
	conn, _ := net.Dial("udp", "localhost"+*logPort)
	buf := []byte(logLine)
	_, err := conn.Write(buf)
	return err
}

func setLogPortRandomly(t *testing.T) {
	port, err := GetRandomUDPPort()
	if err != nil {
		t.Fatalf("Could not locate a random port: %s", err)
	}
	*logPort = ":" + strconv.Itoa(port)
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
