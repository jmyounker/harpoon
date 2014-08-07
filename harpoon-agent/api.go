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
	registry *registry

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

	mux.Put("/containers/:id", http.HandlerFunc(api.handleCreate))
	mux.Get("/containers/:id", http.HandlerFunc(api.handleGet))
	mux.Del("/containers/:id", http.HandlerFunc(api.handleDestroy))
	mux.Post("/containers/:id/heartbeat", http.HandlerFunc(api.handleHeartbeat))
	mux.Post("/containers/:id/start", http.HandlerFunc(api.handleStart))
	mux.Post("/containers/:id/stop", http.HandlerFunc(api.handleStop))
	// TODO(jmy): Uncomment this when we've decided on the interface's final from.
	// mux.Get("/containers/:id/log", http.HandlerFunc(api.handleLog))
	mux.Get("/containers", http.HandlerFunc(api.handleList))

	mux.Get("/resources", http.HandlerFunc(api.handleResources))

	return api
}

func (a *api) Enable() {
	a.Lock()
	defer a.Unlock()

	a.enabled = true // TODO(pb): this is never used
}

func (a *api) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get(":id")

	container, ok := a.registry.Get(id)
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

	if ok := a.registry.Register(container); !ok {
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
}

func (a *api) handleStop(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get(":id")

	container, ok := a.registry.Get(id)
	if !ok {
		http.Error(w, "", http.StatusNotFound)
		return
	}

	if err := container.Stop(); err != nil {
		log.Printf("[%s] stop: %s", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (a *api) handleStart(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get(":id")

	container, ok := a.registry.Get(id)
	if !ok {
		http.Error(w, "", http.StatusNotFound)
		return
	}

	if err := container.Start(); err != nil {
		log.Printf("[%s] start: %s", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (a *api) handleDestroy(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get(":id")

	container, ok := a.registry.Get(id)
	if !ok {
		http.Error(w, "", http.StatusNotFound)
		return
	}

	if err := container.Destroy(); err != nil {
		log.Printf("[%s] destroy: %s", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a.registry.Remove(id)

	w.WriteHeader(http.StatusNoContent)
}

func (a *api) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var heartbeat agent.Heartbeat
	if err := json.NewDecoder(r.Body).Decode(&heartbeat); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	container, ok := a.registry.Get(r.URL.Query().Get(":id"))
	if !ok {
		// Received heartbeat from a container we don't know about. That's
		// relatively bad news: issue a stern rebuke.
		json.NewEncoder(w).Encode(&agent.HeartbeatReply{Want: "FORCEDOWN"})
		return
	}

	want := container.Heartbeat(heartbeat)

	json.NewEncoder(w).Encode(&agent.HeartbeatReply{Want: want})
}

func (a *api) handleContainerStream(_ string, enc *eventsource.Encoder, stop <-chan bool) {
	statec := make(chan agent.ContainerInstance)

	a.registry.Notify(statec)
	defer a.registry.Stop(statec)

	b, err := json.Marshal(a.registry.Instances())
	if err != nil {
		return
	}

	enc.Encode(eventsource.Event{
		Data: b,
	})

	for {
		select {
		case <-stop:
			return
		case state := <-statec:
			b, err := json.Marshal([]agent.ContainerInstance{state})
			if err != nil {
				return
			}

			enc.Encode(eventsource.Event{
				Data: b,
			})
		}
	}
}

func (a *api) handleList(w http.ResponseWriter, r *http.Request) {
	if isStreamAccept(r.Header.Get("Accept")) {
		eventsource.Handler(a.handleContainerStream).ServeHTTP(w, r)
		return
	}

	json.NewEncoder(w).Encode(a.registry.Instances())
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

	container, ok := a.registry.Get(id)
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
		logLines := make(chan string, 2000)
		container.Logs().Notify(logLines)
		defer container.Logs().Stop(logLines)
		for line := range logLines {
			if _, err := w.Write([]byte(line)); err != nil {
				return
			}
		}
		return
	}

	for _, line := range container.Logs().Last(history) {
		if _, err := w.Write([]byte(line)); err != nil {
			return
		}
	}
}

func (a *api) handleResources(w http.ResponseWriter, r *http.Request) {
	volumes := make([]string, 0, len(configuredVolumes))

	for vol := range configuredVolumes {
		volumes = append(volumes, vol)
	}

	var reservedMem, reservedCPU float64

	for _, instance := range a.registry.Instances() {
		reservedMem += float64(instance.Config.Resources.Memory)
		reservedCPU += float64(instance.Config.Resources.CPUs)
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
