package agentrepr_test

import (
	"io/ioutil"
	"log"
	"os"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-scheduler/agentrepr"

	"testing"
	"time"
)

func TestSubscribeUnsubscribe(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		c  = agentrepr.NewFakeClient(t, "foo")
		r  = agentrepr.New(c)
		ch = make(chan map[string]agent.StateEvent, 1)
	)

	r.Subscribe(ch)
	<-ch

	c.Put("bar", agent.ContainerConfig{})

	<-ch // I don't know why this duplicate happens

	if want, have := agent.ContainerStatusCreated, (<-ch)["foo"].Containers["bar"].ContainerStatus; want != have {
		t.Errorf("want %q, have %q", want, have)
	}

	r.Unsubscribe(ch)

	select {
	case <-ch:
		t.Errorf("bad receive")
	default:
	}
}

func TestSchedule(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	var (
		c = agentrepr.NewFakeClient(t, "foo")
		r = agentrepr.New(c)
	)

	time.Sleep(time.Millisecond)

	if err := r.Schedule("bar", agent.ContainerConfig{}); err != nil {
		t.Error(err)
	}

	s, ok := c.Get("bar")
	if !ok {
		t.Error("container doesn't exist")
	}
	if want, have := agent.ContainerStatusRunning, s; want != have {
		t.Errorf("want %q, have %q", want, have)
	}
}

func TestUnschedule(t *testing.T) {
	var (
		c = agentrepr.NewFakeClient(t, "foo")
		r = agentrepr.New(c)
	)

	c.Put("bar", agent.ContainerConfig{})
	c.Start("bar")

	time.Sleep(time.Millisecond)

	if want, have := agent.ContainerStatusRunning, r.Snapshot()["foo"].Containers["bar"].ContainerStatus; want != have {
		t.Errorf("want %q, have %q", want, have)
	}

	if err := r.Unschedule("bar"); err != nil {
		t.Error(err)
	}

	if _, ok := c.Get("bar"); ok {
		t.Error("container exists")
	}
}

// Receiving an update for a container which is outstanding
// after receiving the wanted state of a container
// leads to race condition between the channels "updatec" and "succesc"
// if updatec runs first there's a deadlock trying to singal outstanding
// container
func TestNoDeadlockAfterSchedule(t *testing.T) {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Lmicroseconds | log.Lshortfile)
	agentrepr.Debugf = log.Printf

	var (
		c = agentrepr.NewFakeClient(t, "foo")
		r = agentrepr.New(c)
	)

	time.Sleep(time.Millisecond)

	if err := r.Schedule("bar", agent.ContainerConfig{}); err != nil {
		t.Error(err)
	}

	c.Fail("bar")

	snapshotc := make(chan map[string]agent.StateEvent)
	go func() {
		snapshotc <- r.Snapshot()
	}()

	timeout := time.After(time.Second * 2)

	select {
	case <-snapshotc:
	case <-timeout:
		t.Error("Timeout")
	}
}
