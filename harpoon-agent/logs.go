package main

// Provide log management for containers.
//
// containerLogs implement log buffering, listening, and log retrieval for a
// single container.
//
// ringBuffer implements rolling log storage and retrieval of the last N log
// lines.

import (
	"container/ring"
	"log"
	"net"
	"regexp"
	"sync"
)

const (
	logBufferSize        = 10000 // lines
	averageLogLineLength = 120   // chars
)

type containerLog struct {
	addc    chan string
	lastc   chan logLast
	notifyc chan chan string
	stopc   chan chan string
	quitc   chan chan struct{}
}

// newContainerLog allocates and initializes a ring-buffered log structure for
// a single active container.
func newContainerLog(bufferSize int) *containerLog {
	cl := &containerLog{
		addc:    make(chan string),
		lastc:   make(chan logLast),
		notifyc: make(chan chan string),
		stopc:   make(chan chan string),
		quitc:   make(chan chan struct{}),
	}
	go cl.loop(bufferSize)
	return cl
}

type logLast struct {
	count int           // supplied by caller
	last  chan []string // passes result to caller
}

// addLogLine feeds a log entry into a log buffer and notifies all listeners.
//
// METRICS:
//  # number lines received
//  # number log lines flushed
//  # of notifications delivered
//  # number notifications dropped
//
// If the number of dropped notifications goes up then a listener is not
// consuming notifications fast enough. Look for a stalled listener.
func (cl *containerLog) addLogLine(line string) {
	cl.addc <- line
}

// last retrieves the n last log lines from containerID, returning them in the
// order from oldest to newest, i.e. []string{oldest, newer, ..., newest}. The
// call is is idempotent.
func (cl *containerLog) last(n int) []string {
	msg := logLast{count: n, last: make(chan []string)}
	cl.lastc <- msg
	return <-msg.last
}

// notify subscribes a listener to a container. New log lines are sent to all
// linecs in the notifications set. A linec does not receive messages
// while it is blocked. All of those messages are lost like tears in the rain.
func (cl *containerLog) notify(linec chan string) {
	cl.notifyc <- linec
}

// stop removes the linec from the notifications set.
func (cl *containerLog) stop(linec chan string) {
	cl.stopc <- linec
}

// exit causes terminates the loop() cleanly
func (cl *containerLog) exit() {
	q := make(chan struct{})
	cl.quitc <- q
	<-q
}

// loop processes incoming commands
func (cl *containerLog) loop(bufferSize int) {
	var (
		entries       = newRingBuffer(bufferSize)
		notifications = make(map[chan string]struct{})
	)
	for {
		select {
		case line := <-cl.addc:
			entries.insert(line)
			for linec := range notifications {
				select {
				case linec <- line:
					incLogDeliverableLines(1)
				default:
					incLogUndeliveredLines(1)
				}
			}
		case msg := <-cl.lastc:
			msg.last <- entries.last(msg.count)
		case linec := <-cl.notifyc:
			notifications[linec] = struct{}{}
		case linec := <-cl.stopc:
			close(linec)
			delete(notifications, linec)
		case q := <-cl.quitc:
			for linec := range notifications {
				close(linec)
			}
			close(q)
			return
		}
	}
}

// receiveLogs opens udp port 3334, listens for incoming log messages, and
// then feeds these into the appropriate buffers.
func receiveLogs(r *registry, logAddr string) {
	laddr, err := net.ResolveUDPAddr("udp", logAddr)
	if err != nil {
		log.Fatal(err)
	}

	ln, err := net.ListenUDP("udp", laddr)
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	ln.SetReadBuffer(logBufferSize * averageLogLineLength)

	var buf = make([]byte, maxLogLineLength+maxContainerIDLength)

	// All log lines should start with the pattern container[FOO] where FOO
	// is the container ID.
	containsPtrn := regexp.MustCompile(`container\[([^\]]+)]`)

	for {
		n, addr, err := ln.ReadFromUDP(buf)
		if err != nil {
			log.Printf("logs: while reading from port: %s", err)
			return
		}
		incLogReceivedLines(1)

		line := string(buf[:n])
		matches := containsPtrn.FindStringSubmatch(line)
		if len(matches) != 2 {
			incLogUnparsableLines(1)
			log.Printf("logs: %s: message to unknown container: %s", addr, line)
			continue
		}

		container, ok := r.get(matches[1])
		if !ok {
			incLogUnroutableLines(1)
			log.Printf("logs: %s: message to unknown container: %s", addr, line)
			continue
		}

		container.Logs().addLogLine(line)
	}
}

// ringBuffer that allows you to retrieve the last n records. Retrieval calls are idempotent.
type ringBuffer struct {
	sync.Mutex
	elements *ring.Ring
	length   int
}

// newRingBuffer creates a new ring buffer of the specified size.
func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{elements: ring.New(size), length: size}
}

// insert a message into the ring buffer.
func (b *ringBuffer) insert(x string) {
	b.Lock()
	defer b.Unlock()
	b.elements.Value = x
	b.elements = b.elements.Next()
}

// Last returns the last count entries from the ring buffer. These
// are returned from oldest to newest, i.e. []string{oldest, ..., newest}.
// It will never return more entries than the ringBuffer can hold, although
// it may return fewer if the ring buffer has fewer entries than were
// requested.
func (b *ringBuffer) last(count int) []string {
	count = min(count, b.length)
	results := make([]string, 0, count)

	b.Lock()
	defer b.Unlock()

	prev := b.elements
	for i := 0; i < count; i++ {
		prev = prev.Prev()
		if prev.Value == nil {
			break
		}
		results = append(results, prev.Value.(string))
	}

	return reverse(results)
}

// reverse reverses the oder of a slice destructively.
func reverse(x []string) []string {
	for i := 0; i < len(x)/2; i++ {
		x[i], x[len(x)-i-1] = x[len(x)-i-1], x[i]
	}
	return x
}

// min returns the minimum of two ints.
func min(x int, y int) int {
	if x < y {
		return x
	}
	return y
}
