package api_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/soundcloud/harpoon/harpoon-scheduler/api"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

// https://github.com/soundcloud/harpoon/pull/107
func TestUnscheduleFailed(t *testing.T) {
	var (
		i = agent.ContainerInstance{ContainerStatus: agent.ContainerStatusFailed}
		c = map[string]agent.ContainerInstance{"bar": i}
		e = agent.StateEvent{Containers: c}
		p = fakeProxy{"foo": e}
		s = &fakeJobScheduler{}
		h = api.NewHandler(p, s)
	)

	w := httptest.NewRecorder()
	r, err := http.NewRequest("PUT", "http://cats.biz"+api.APIVersionPrefix+api.APIUnschedulePath+"/bar", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s %s", r.Method, r.URL.String())

	h.ServeHTTP(w, r)

	t.Logf(w.Body.String())

	if want, have := int32(1), atomic.LoadInt32(&s.unschedules); want != have {
		t.Errorf("want %d unschedule(s), have %d", want, have)
	}
}

type fakeProxy map[string]agent.StateEvent

func (p fakeProxy) Snapshot() map[string]agent.StateEvent {
	return map[string]agent.StateEvent(p)
}

type fakeJobScheduler struct {
	schedules   int32
	unschedules int32
	snapshots   int32
}

func (s *fakeJobScheduler) Schedule(configstore.JobConfig) error {
	atomic.AddInt32(&s.schedules, 1)
	return nil
}

func (s *fakeJobScheduler) Unschedule(jobConfigHash string) error {
	atomic.AddInt32(&s.unschedules, 1)
	return nil
}

func (s fakeJobScheduler) Snapshot() map[string]configstore.JobConfig {
	atomic.AddInt32(&s.snapshots, 1)
	return map[string]configstore.JobConfig{}
}
