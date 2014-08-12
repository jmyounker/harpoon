package agent

import (
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestMockAgent(t *testing.T) {
	//log.SetFlags(log.Lmicroseconds)
	log.SetOutput(ioutil.Discard)

	var (
		a = NewMock()
		s = httptest.NewServer(a)
	)

	defer s.Close()

	r := strings.NewReplacer(":id", "foobar")

	for _, tuple := range []struct {
		method, path string
		count        *int32
	}{
		{"GET", APIVersionPrefix + r.Replace(APIListContainersPath), &a.listContainersCount},
		{"PUT", APIVersionPrefix + r.Replace(APICreateContainerPath), &a.createContainerCount},
		{"GET", APIVersionPrefix + r.Replace(APIGetContainerPath), &a.getContainerCount},
		{"DELETE", APIVersionPrefix + r.Replace(APIDestroyContainerPath), &a.destroyContainerCount},
		{"POST", APIVersionPrefix + r.Replace(APIStartContainerPath), &a.startContainerCount},
		{"POST", APIVersionPrefix + r.Replace(APIStopContainerPath), &a.stopContainerCount},
		{"GET", APIVersionPrefix + r.Replace(APIGetContainerLogPath), &a.getContainerLogCount},
		{"GET", APIVersionPrefix + r.Replace(APIGetResourcesPath), &a.getResourcesCount},
	} {
		method, path, count := tuple.method, tuple.path, tuple.count
		pre := atomic.LoadInt32(count)

		req, err := http.NewRequest(method, s.URL+path, nil)
		if err != nil {
			t.Errorf("%s %s: %s", method, path, err)
			continue
		}

		if _, err = http.DefaultClient.Do(req); err != nil {
			t.Errorf("%s %s: %s", method, path, err)
			continue
		}

		post := atomic.LoadInt32(count)

		if delta := post - pre; delta != 1 {
			t.Errorf("%s %s: handler didn't get called (pre-count %d, post-count %d)", method, path, pre, post)
		}

		t.Logf("%s %s: OK (%d -> %d)", method, path, pre, post)
	}
}
