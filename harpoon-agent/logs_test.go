package main

import (
	"io/ioutil"
	"log"
	"reflect"
	"testing"
	"time"
)

func TestLastRetrievesLastLogLines(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	cl := newContainerLog(3)
	cl.addLogLine("m1")

	ExpectArraysEqual(t, cl.last(1), []string{"m1"})
}

func TestListenersReceiveMessages(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		cl      = newContainerLog(3)
		logSink = make(chan string, 1) // can't block
	)

	cl.notify(logSink)
	cl.addLogLine("m1")

	ExpectMessage(t, logSink, "m1")
}

func TestBlockedChannelsAreSkipped(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		cl      = newContainerLog(3)
		logSink = make(chan string)
	)

	cl.notify(logSink)
	cl.addLogLine("m1")

	ExpectNoMessage(t, logSink)
}

func TestListenerShouldReceivesAllMessagesOnChannel(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		cl      = newContainerLog(3)
		logSink = make(chan string, 2)
	)

	cl.notify(logSink)
	cl.addLogLine("m1")
	cl.addLogLine("m2")

	ExpectMessage(t, logSink, "m1")
	ExpectMessage(t, logSink, "m2")
}

func TestMessagesShouldBroadcastToAllListeners(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		cl       = newContainerLog(3)
		logSink1 = make(chan string, 2)
		logSink2 = make(chan string, 2)
	)

	cl.notify(logSink1)
	cl.notify(logSink2)
	cl.addLogLine("m1")

	ExpectMessage(t, logSink1, "m1")
	ExpectMessage(t, logSink2, "m1")
}

func TestRemovedListenersDoNotReceiveMessages(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		cl       = newContainerLog(3)
		logSink1 = make(chan string, 2)
		logSink2 = make(chan string, 2)
	)

	cl.notify(logSink1)
	cl.notify(logSink2)
	cl.stop(logSink2)
	cl.addLogLine("m1")

	ExpectMessage(t, logSink1, "m1")
	ExpectNoMessage(t, logSink2)
}

func TestKillingContainerUnblocksListeners(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		cl                 = newContainerLog(3)
		logSink            = make(chan string, 1)
		receiverTerminated = make(chan struct{})
	)

	go func() {
		select {
		case <-logSink:
		case <-time.After(10 * time.Millisecond):
			t.Errorf("Blocked task never received an unblocking")
		}
		close(receiverTerminated)
	}()

	cl.notify(logSink)
	cl.exit()

	select {
	case <-receiverTerminated:
	case <-time.After(10 * time.Millisecond):
		t.Errorf("Receiver never terminated")
	}
}

func ExpectMessage(t *testing.T, logSink chan string, expected string) {
	msg := <-logSink
	if msg != expected {
		t.Errorf("Received %q when expecting %q", msg, expected)
	}
}

func ExpectNoMessage(t *testing.T, logSink chan string) {
	select {
	case logLine := <-logSink:
		if logLine != "" {
			t.Errorf("Received log line %q when we should have received nothing", logLine)
		}
	default:
		// Happy path!
	}
}

func TestEmptyRingBufferHasNoLastElements(t *testing.T) {
	rb := newRingBuffer(3)
	ExpectArraysEqual(t, rb.last(2), []string{})
}

func TestRingBufferWithSomethingReturnsSomething(t *testing.T) {
	rb := newRingBuffer(3)
	rb.insert("m1")
	ExpectArraysEqual(t, rb.last(1), []string{"m1"})
}

func TestRingBufferOnlyReturnsNumberOfResultsPresent(t *testing.T) {
	// Checks that nil was used to limit number returned.
	rb := newRingBuffer(3)
	rb.insert("m1")
	ExpectArraysEqual(t, rb.last(2), []string{"m1"})
}

func TestLastOnlyReturnsTheRequestedNumberOfElements(t *testing.T) {
	// Checks that index was used to limit number returned.
	rb := newRingBuffer(3)
	rb.insert("m1")
	rb.insert("m2")
	ExpectArraysEqual(t, rb.last(1), []string{"m2"})
}

func TestLastReturnsResultsFromOldestToNewest(t *testing.T) {
	rb := newRingBuffer(3)
	rb.insert("m1")
	rb.insert("m2")
	ExpectArraysEqual(t, rb.last(2), []string{"m1", "m2"})
}

func TestRingBufferWithCapacityNReallyHoldsNRecords(t *testing.T) {
	rb := newRingBuffer(3)
	rb.insert("m1")
	rb.insert("m2")
	rb.insert("m3")
	ExpectArraysEqual(t, rb.last(3), []string{"m1", "m2", "m3"})
}

func TestRingBufferWithCapacityNReallyHoldsOnlyNRecords(t *testing.T) {
	rb := newRingBuffer(3)
	rb.insert("m1")
	rb.insert("m2")
	rb.insert("m3")
	rb.insert("m4")
	ExpectArraysEqual(t, rb.last(3), []string{"m2", "m3", "m4"})
}

func TestLastLimitsRetrievalToTheRingBufferSize(t *testing.T) {
	rb := newRingBuffer(3)
	rb.insert("m1")
	rb.insert("m2")
	rb.insert("m3")
	rb.insert("m4")
	ExpectArraysEqual(t, rb.last(4), []string{"m2", "m3", "m4"})
}

func TestReverse(t *testing.T) {
	ExpectArraysEqual(t, reverse([]string{}), []string{})
	ExpectArraysEqual(t, reverse([]string{"1"}), []string{"1"})
	ExpectArraysEqual(t, reverse([]string{"1", "2"}), []string{"2", "1"})
	ExpectArraysEqual(t, reverse([]string{"1", "2", "3"}), []string{"3", "2", "1"})
}

func TestMin(t *testing.T) {
	ExpectEqual(t, min(1, 2), 1)
	ExpectEqual(t, min(2, 1), 1)
	ExpectEqual(t, min(1, 1), 1)
}

func ExpectArraysEqual(t *testing.T, x []string, y []string) {
	if !reflect.DeepEqual(x, y) {
		t.Errorf("%q != %q", x, y)
	}
}

func ExpectEqual(t *testing.T, x int, y int) {
	if x != y {
		t.Errorf("%q != %q", x, y)
	}
}
