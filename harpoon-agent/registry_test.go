package main

import (
	"io/ioutil"
	"log"
	"testing"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestMessagesGetWrittenToLogs(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	registry := newRegistry()

	createReceiveLogsFixture(t, registry)

	c := newFakeContainer("123", "", volumes{}, agent.ContainerConfig{}, false, nil)
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
	expectArraysEqual(t, logLines, []string{"container[123] m1", "container[123] m2"})
}

func TestLogRoutingOfDefectiveMessages(t *testing.T) {
	registry := newRegistry()

	createReceiveLogsFixture(t, registry)

	c := newFakeContainer("123", "", volumes{}, agent.ContainerConfig{}, false, nil)
	registry.register(c)

	linec := make(chan string, 10) // Plenty of room before anything gets dropped
	c.Logs().notify(linec)

	sendLog("ilj;irtr") // Should not be received

	// TODO(jmy): In the future make sure that "unroutable message" counter goes up.
	// For now a timeout is a workable substitute for verifying that a log message was
	// not routed.  It's better than nothing.
	expectNoLogLines(t, linec, 100*time.Millisecond)
}
