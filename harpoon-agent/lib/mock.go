package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/bernerdschaefer/eventsource"
	"github.com/julienschmidt/httprouter"
)

// Mock implements an in-memory Agent for tests.
type Mock struct {
	*httprouter.Router

	sync.RWMutex
	instances   map[string]ContainerInstance
	subscribers map[chan ContainerInstance]struct{}

	listContainersCount   int32
	createContainerCount  int32
	getContainerCount     int32
	destroyContainerCount int32
	startContainerCount   int32
	stopContainerCount    int32
	getContainerLogCount  int32
	getResourcesCount     int32
}

// NewMock returns a new Mock, designed to be passed to httptest.NewServer.
func NewMock() *Mock {
	m := &Mock{
		Router:      httprouter.New(),
		instances:   map[string]ContainerInstance{},
		subscribers: map[chan ContainerInstance]struct{}{},
	}

	m.Router.GET(APIVersionPrefix+APIListContainersPath, m.listContainers)
	m.Router.PUT(APIVersionPrefix+APICreateContainerPath, m.createContainer)
	m.Router.GET(APIVersionPrefix+APIGetContainerPath, m.getContainer)
	m.Router.DELETE(APIVersionPrefix+APIDestroyContainerPath, m.destroyContainer)
	m.Router.POST(APIVersionPrefix+APIStartContainerPath, m.startContainer)
	m.Router.POST(APIVersionPrefix+APIStopContainerPath, m.stopContainer)
	m.Router.GET(APIVersionPrefix+APIGetContainerLogPath, m.getContainerLog)
	m.Router.GET(APIVersionPrefix+APIGetResourcesPath, m.getResources)

	return m
}

func (m *Mock) subscribe(c chan ContainerInstance) {
	m.Lock()
	defer m.Unlock()

	m.subscribers[c] = struct{}{}
}

func (m *Mock) unsubscribe(c chan ContainerInstance) {
	m.Lock()
	defer m.Unlock()

	delete(m.subscribers, c)
}

func broadcastContainerInstance(dst map[chan ContainerInstance]struct{}, containerInstance ContainerInstance) {
	for c := range dst {
		select {
		case c <- containerInstance:
		default:
		}
	}
}

func (m *Mock) getContainerInstances() map[string]ContainerInstance {
	m.RLock()
	defer m.RUnlock()

	return m.instances
}

func (m *Mock) listContainers(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	defer atomic.AddInt32(&m.listContainersCount, 1)

	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		eventsource.Handler(func(lastID string, enc *eventsource.Encoder, stop <-chan bool) {
			changec := make(chan ContainerInstance)

			m.subscribe(changec)
			defer m.unsubscribe(changec)

			buf, _ := json.Marshal(m.getContainerInstances())
			enc.Encode(eventsource.Event{Data: buf})

			for {
				select {
				case <-stop:
					log.Printf("mockAgent getContainerEvents: HTTP request closed")
					return
				case instance := <-changec:
					buf, _ = json.Marshal(map[string]ContainerInstance{instance.ID: instance})
					enc.Encode(eventsource.Event{Data: buf})
				}
			}
		}).ServeHTTP(w, r)
		return
	}

	json.NewEncoder(w).Encode(m.getContainerInstances())
}

func (m *Mock) getContainerEvents(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	log.Printf("mockAgent getContainerEvents: stream started")
	defer log.Printf("mockAgent getContainerEvents: stream stopped")

	closeNotifier, ok := w.(http.CloseNotifier)
	if !ok {
		panic("ResponseWriter not CloseNotifier")
	}

	var (
		enc     = eventsource.NewEncoder(w)
		closec  = closeNotifier.CloseNotify()
		changec = make(chan ContainerInstance)
	)

	m.subscribe(changec)
	defer m.unsubscribe(changec)

	buf, err := json.Marshal(m.getContainerInstances())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "text/event-stream")

	enc.Encode(eventsource.Event{Data: buf})

	for {
		select {
		case instance := <-changec:
			buf, _ := json.Marshal(map[string]ContainerInstance{instance.ID: instance})
			enc.Encode(eventsource.Event{Data: buf})
		case <-closec:
			log.Printf("mockAgent getContainerEvents: HTTP request closed")
			return
		}
	}
}

func (m *Mock) createContainer(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	defer atomic.AddInt32(&m.createContainerCount, 1)

	id := p.ByName("id")
	if id == "" {
		http.Error(w, fmt.Sprintf("%q required", "id"), http.StatusBadRequest)
		return
	}

	var config ContainerConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	instance := ContainerInstance{
		ID:     id,
		Status: ContainerStatusRunning, // PUT also starts
		Config: config,
	}

	// PUT also starts.
	func() {
		m.Lock()
		defer m.Unlock()
		m.instances[id] = instance
		broadcastContainerInstance(m.subscribers, instance)
	}()

	w.WriteHeader(http.StatusCreated)
}

func (m *Mock) getContainer(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	defer atomic.AddInt32(&m.getContainerCount, 1)

	id := p.ByName("id")
	if id == "" {
		http.Error(w, fmt.Sprintf("%q required", "id"), http.StatusBadRequest)
		return
	}

	m.RLock()
	defer m.RUnlock()

	containerInstance, ok := m.instances[id]
	if !ok {
		http.Error(w, fmt.Sprintf("%q not present", id), http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(containerInstance)
}

func (m *Mock) destroyContainer(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	defer atomic.AddInt32(&m.destroyContainerCount, 1)

	id := p.ByName("id")
	if id == "" {
		http.Error(w, fmt.Sprintf("%q required", "id"), http.StatusBadRequest)
		return
	}

	m.Lock()
	defer m.Unlock()

	containerInstance, ok := m.instances[id]
	if !ok {
		http.Error(w, fmt.Sprintf("%q not present", id), http.StatusNotFound)
		return
	}

	switch containerInstance.Status {
	case ContainerStatusFailed, ContainerStatusFinished:
		delete(m.instances, id)
		containerInstance.Status = ContainerStatusDeleted
		broadcastContainerInstance(m.subscribers, containerInstance)
		w.WriteHeader(http.StatusOK)
		return

	default:
		http.Error(w, fmt.Sprintf("%q not in a finished state, currently %s", id, containerInstance.Status), http.StatusNotFound)
		return
	}
}

func (m *Mock) startContainer(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	defer atomic.AddInt32(&m.startContainerCount, 1)

	id := p.ByName("id")
	if id == "" {
		http.Error(w, fmt.Sprintf("%q required", "id"), http.StatusBadRequest)
		return
	}

	http.Error(w, fmt.Sprintf("start not yet implemented"), http.StatusNotImplemented)
}

func (m *Mock) stopContainer(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	defer atomic.AddInt32(&m.stopContainerCount, 1)

	id := p.ByName("id")
	if id == "" {
		http.Error(w, fmt.Sprintf("%q required", "id"), http.StatusBadRequest)
		return
	}

	m.Lock()
	defer m.Unlock()

	containerInstance, ok := m.instances[id]
	if !ok {
		http.Error(w, fmt.Sprintf("%q unknown; can't stop", id), http.StatusNotFound)
		return
	}

	if containerInstance.Status != ContainerStatusRunning {
		http.Error(w, fmt.Sprintf("%q not running (%s), can't stop", id, containerInstance.Status), http.StatusBadRequest)
		return
	}

	containerInstance.Status = ContainerStatusFinished

	w.WriteHeader(http.StatusAccepted) // "[Stop] returns immediately with 202 status."

	go func() {
		m.Lock()
		defer m.Unlock()
		m.instances[id] = containerInstance
		broadcastContainerInstance(m.subscribers, containerInstance)
	}()
}

func (m *Mock) getContainerLog(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	defer atomic.AddInt32(&m.getContainerLogCount, 1)

	http.Error(w, fmt.Sprintf("log not yet implemented"), http.StatusNotImplemented)
}

func (m *Mock) getResources(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	defer atomic.AddInt32(&m.getResourcesCount, 1)

	json.NewEncoder(w).Encode(HostResources{
		Memory:  TotalReserved{Total: 32768, Reserved: 16384},
		CPUs:    TotalReserved{Total: 8, Reserved: 1},
		Storage: TotalReserved{Total: 322122547200, Reserved: 123125031034},
		Volumes: []string{"/data/analytics-kibana", "/data/mysql000", "/data/mysql001"},
	})
}