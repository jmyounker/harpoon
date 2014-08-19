package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

var (
	algorithm = randomChoice
)

type jobScheduler interface {
	schedule(configstore.JobConfig) error
	unschedule(configstore.JobConfig) error
	migrate(from, to configstore.JobConfig) error
}

type snapshotter interface {
	snapshot() map[string]map[string]agent.ContainerInstance
}

type scheduler interface {
	jobScheduler
	snapshotter
}

type realScheduler struct {
	schedc    chan schedJobReq
	unschedc  chan schedJobReq
	migratec  chan migrateJobReq
	snapshotc chan map[string]map[string]agent.ContainerInstance
	quitc     chan chan struct{}
}

var _ scheduler = &realScheduler{}

func newRealScheduler(actual actualBroadcaster, target taskScheduler) *realScheduler {
	s := &realScheduler{
		schedc:    make(chan schedJobReq),
		unschedc:  make(chan schedJobReq),
		migratec:  make(chan migrateJobReq),
		snapshotc: make(chan map[string]map[string]agent.ContainerInstance),
		quitc:     make(chan chan struct{}),
	}

	go s.loop(actual, target)

	return s
}

func (s *realScheduler) schedule(job configstore.JobConfig) error {
	if err := job.Valid(); err != nil {
		return err
	}

	req := schedJobReq{job, make(chan error)}

	s.schedc <- req

	return <-req.err

}

func (s *realScheduler) unschedule(job configstore.JobConfig) error {
	if err := job.Valid(); err != nil {
		return err
	}

	req := schedJobReq{job, make(chan error)}

	s.unschedc <- req

	return <-req.err
}

func (s *realScheduler) migrate(from, to configstore.JobConfig) error {
	if err := from.Valid(); err != nil {
		return err
	}

	if err := to.Valid(); err != nil {
		return err
	}

	req := migrateJobReq{from, to, make(chan error)}

	s.migratec <- req

	return <-req.err
}

func (s *realScheduler) snapshot() map[string]map[string]agent.ContainerInstance {
	return <-s.snapshotc
}

func (s *realScheduler) quit() {
	q := make(chan struct{})
	s.quitc <- q
	<-q
}

func (s *realScheduler) loop(actual actualBroadcaster, target taskScheduler) {
	var (
		updatec = make(chan map[string]map[string]agent.ContainerInstance)
		current = map[string]map[string]agent.ContainerInstance{}
	)

	actual.subscribe(updatec)
	defer actual.unsubscribe(updatec)

	select {
	case current = <-updatec:
	case <-time.After(time.Millisecond):
		panic("misbehaving actual-state broadcaster")
	}

	for {
		select {
		case req := <-s.schedc:
			req.err <- scheduleJob(req.JobConfig, current, target)

		case req := <-s.unschedc:
			req.err <- unscheduleJob(req.JobConfig, current, target)

		case req := <-s.migratec:
			req.err <- migrateJob(req.from, req.to, current, target)

		case current = <-updatec:
		case s.snapshotc <- current:

		case q := <-s.quitc:
			close(q)
			return
		}
	}
}

func scheduleJob(jobConfig configstore.JobConfig, current map[string]map[string]agent.ContainerInstance, target taskScheduler) error {
	specs, err := algorithm(jobConfig, current)
	if err != nil {
		return err
	}

	if len(specs) <= 0 {
		return fmt.Errorf("job contained no tasks")
	}

	incContainersPlaced(len(specs))

	undo := []func(){}

	defer func() {
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i]()
		}
	}()

	for _, spec := range specs {
		if err := target.schedule(spec); err != nil {
			return err
		}

		undo = append(undo, func() { target.unschedule(spec.Endpoint, spec.ContainerID) })
	}

	undo = []func(){}

	return nil
}

func unscheduleJob(jobConfig configstore.JobConfig, current map[string]map[string]agent.ContainerInstance, target taskScheduler) error {
	type tuple struct{ jobName, taskName string }

	var targets = map[string]tuple{}

	for _, taskConfig := range jobConfig.Tasks {
		for i := 0; i < taskConfig.Scale; i++ {
			targets[makeContainerID(jobConfig, taskConfig, i)] = tuple{jobConfig.JobName, taskConfig.TaskName}
		}
	}

	var (
		orig = len(targets)
		undo = []func(){}
	)

	defer func() {
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i]()
		}
	}()

	for endpoint, instances := range current {
		for id, instance := range instances {
			if tuple, ok := targets[id]; ok {
				revertSpec := taskSpec{
					Endpoint:        endpoint,
					JobName:         tuple.jobName,
					TaskName:        tuple.taskName,
					ContainerID:     id,
					ContainerConfig: instance.Config,
				}

				if err := target.unschedule(endpoint, id); err != nil {
					return err
				}

				undo = append(undo, func() { target.schedule(revertSpec) })

				delete(targets, id)
			}
		}
	}

	if len(targets) >= orig {
		return fmt.Errorf("job not scheduled")
	}

	if len(targets) > 0 {
		log.Printf("scheduler: unschedule job: failed to find %d container(s) (%v)", len(targets), targets)
	}

	undo = []func(){}

	return nil
}

func migrateJob(from, to configstore.JobConfig, current map[string]map[string]agent.ContainerInstance, target taskScheduler) error {
	return fmt.Errorf("not yet implemented")
}

func makeContainerID(j configstore.JobConfig, t configstore.TaskConfig, i int) string {
	return fmt.Sprintf("%s-%s:%s-%s:%d", j.JobName, refHash(j), t.TaskName, refHash(t), i)
}

func refHash(v interface{}) string {
	// TODO(pb): need stable encoding, either not-JSON (most likely), or some
	// way of getting stability out of JSON.
	h := md5.New()

	if err := json.NewEncoder(h).Encode(v); err != nil {
		panic(fmt.Sprintf("%s: refHash error: %s", reflect.TypeOf(v), err))
	}

	return fmt.Sprintf("%x", h.Sum(nil))[:7]
}

type schedJobReq struct {
	configstore.JobConfig
	err chan error
}

type migrateJobReq struct {
	from, to configstore.JobConfig
	err      chan error
}
