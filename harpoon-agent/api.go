package main

import (
	"encoding/json"
	"log"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/bernerdschaefer/eventsource"
	"github.com/bmizerany/pat"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type api struct {
	http.Handler
	*registry

	enabled bool
	sync.RWMutex
}

func newAPI(r *registry) *api {
	var (
		mux = pat.New()
		api = &api{
			Handler:  mux,
			registry: r,
		}
	)

	mux.Put("/api/v0/containers/:id", http.HandlerFunc(api.handleCreate))
	mux.Get("/api/v0/containers/:id", http.HandlerFunc(api.handleGet))
	mux.Del("/api/v0/containers/:id", http.HandlerFunc(api.handleDestroy))
	mux.Post("/api/v0/containers/:id/start", http.HandlerFunc(api.handleStart))
	mux.Post("/api/v0/containers/:id/stop", http.HandlerFunc(api.handleStop))
	mux.Get("/api/v0/containers/:id/log", http.HandlerFunc(api.handleLog))
	mux.Get("/api/v0/containers", http.HandlerFunc(api.handleList))
	mux.Get("/api/v0/resources", http.HandlerFunc(api.handleResources))

	return api
}

func (a *api) enable() {
	a.Lock()
	defer a.Unlock()

	a.enabled = true // TODO(pb): this is never used
}

func (a *api) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get(":id")

	container, ok := a.registry.get(id)
	if !ok {
		http.Error(w, "", http.StatusNotFound)
		return
	}

	buf, err := json.MarshalIndent(container, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(buf)
}

func (a *api) handleCreate(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get(":id")

	if id == "" {
		http.Error(w, "no id specified", http.StatusBadRequest)
		return
	}

	var config agent.ContainerConfig

	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	container := newContainer(id, config)

	if ok := a.registry.register(container); !ok {
		http.Error(w, "already exists", http.StatusConflict)
		return
	}

	if err := container.Create(); err != nil {
		log.Printf("[%s] create: %s", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := container.Start(); err != nil {
		log.Printf("[%s] create, start: %s", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("created OK"))
}

func (a *api) handleStop(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get(":id")

	container, ok := a.registry.get(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if container.Instance().ContainerStatus == agent.ContainerStatusFinished {
		log.Printf("[%s] start: already stopped", id)
		http.Error(w, "already stopped", http.StatusConflict)
		return
	}

	if err := container.Stop(); err != nil {
		log.Printf("[%s] stop: %s", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("stop accepted"))
}

func (a *api) handleStart(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get(":id")

	container, ok := a.registry.get(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if container.Instance().ContainerStatus == agent.ContainerStatusRunning {
		log.Printf("[%s] start: already running", id)
		http.Error(w, "already running", http.StatusConflict)
		return
	}

	if err := container.Start(); err != nil {
		log.Printf("[%s] start: %s", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("start accepted"))
}

func (a *api) handleDestroy(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get(":id")

	container, ok := a.registry.get(id)
	if !ok {
		http.Error(w, "", http.StatusNotFound)
		return
	}

	if err := container.Destroy(); err != nil {
		log.Printf("[%s] destroy: %s", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a.registry.remove(id)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("destroy OK"))
}

func (a *api) handleContainerStream(_ string, enc *eventsource.Encoder, stop <-chan bool) {
	statec := make(chan agent.ContainerInstance)

	a.registry.notify(statec)
	defer a.registry.stop(statec)

	b, err := json.Marshal(a.registry.instances())
	if err != nil {
		log.Printf("container stream: fatal error: %s", err)
		return
	}

	if err := enc.Encode(eventsource.Event{Data: b}); err != nil {
		log.Printf("container stream: fatal error: %s", err)
		return
	}

	for {
		select {
		case <-stop:
			return
		case state := <-statec:
			b, err := json.Marshal(map[string]agent.ContainerInstance{state.ID: state})
			if err != nil {
				log.Printf("container stream: fatal error: %s", err)
				return
			}

			if err := enc.Encode(eventsource.Event{Data: b}); err != nil {
				log.Printf("container stream: non-fatal error: %s", err)
			}
		}
	}
}

func (a *api) handleList(w http.ResponseWriter, r *http.Request) {
	if isStreamAccept(r.Header.Get("Accept")) {
		eventsource.Handler(a.handleContainerStream).ServeHTTP(w, r)
		return
	}

	json.NewEncoder(w).Encode(a.registry.instances())
}

func isStreamAccept(accept string) bool {
	for _, a := range strings.Split(accept, ",") {
		mediatype, _, err := mime.ParseMediaType(a)
		if err != nil {
			continue
		}

		if mediatype == "text/event-stream" {
			return true
		}
	}

	return false
}

func (a *api) handleLog(w http.ResponseWriter, r *http.Request) {
	var (
		id         = r.URL.Query().Get(":id")
		rawHistory = r.URL.Query().Get("history")
	)

	if rawHistory == "" {
		rawHistory = "10"
	}

	container, ok := a.registry.get(id)
	if !ok {
		http.Error(w, "", http.StatusNotFound)
		return
	}

	history, err := strconv.Atoi(rawHistory)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if isStreamAccept(r.Header.Get("Accept")) {
		eventsource.Handler(func(_ string, enc *eventsource.Encoder, stop <-chan bool) {
			a.streamLog(container.Logs(), enc, stop)
		}).ServeHTTP(w, r)
		return
	}

	json.NewEncoder(w).Encode(container.Logs().last(history))
}

func (a *api) streamLog(logs *containerLog, enc *eventsource.Encoder, stop <-chan bool) {
	// logs.Notify does not write to blocked channels, so the channel has to
	// be buffered. The capacity is chosen so that a burst of log lines won't
	// immediately result in a loss of data during large surge of incoming log
	// lines.
	linec := make(chan string, logBufferSize)

	logs.notify(linec)
	defer logs.stop(linec)

	for {
		select {
		case <-stop:
			return

		case line := <-linec:
			b, err := json.Marshal([]string{line})
			if err != nil {
				log.Printf("log stream: fatal error: %s", err)
				return
			}

			if err = enc.Encode(eventsource.Event{Data: b}); err != nil {
				log.Printf("log stream: non-fatal error: %s", err)
			}
		}
	}
}

func (a *api) handleResources(w http.ResponseWriter, r *http.Request) {
	volumes := make([]string, 0, len(configuredVolumes))

	for vol := range configuredVolumes {
		volumes = append(volumes, vol)
	}

	var reservedMem, reservedCPU float64

	for _, instance := range a.registry.instances() {
		reservedMem += float64(instance.ContainerConfig.Resources.Memory)
		reservedCPU += float64(instance.ContainerConfig.Resources.CPUs)
	}

	json.NewEncoder(w).Encode(&agent.HostResources{
		Memory: agent.TotalReserved{
			Total:    float64(agentTotalMem),
			Reserved: reservedMem,
		},
		CPUs: agent.TotalReserved{
			Total:    float64(agentTotalCPU),
			Reserved: reservedCPU,
		},
		Volumes: volumes,
	})
}
