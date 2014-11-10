// Package api implements REST-y handlers for interacting with the scheduler.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

const (
	// APIVersionPrefix for all API endpoints.
	APIVersionPrefix = "/api/v0"

	// APISchedulePath for schedule calls.
	APISchedulePath = "/schedule"

	// APIUnschedulePath for schedule calls.
	APIUnschedulePath = "/unschedule"

	// APIProxyPath to get the actual state of the scheduling domain.
	APIProxyPath = "/proxy"

	// APIRegistryPath to get the desired state of the scheduling domain.
	APIRegistryPath = "/registry"
)

type handler struct {
	Proxy
	JobScheduler
}

// Proxy captures the methods to get the actual state of the scheduling
// domain.
type Proxy interface {
	Snapshot() map[string]agent.StateEvent
}

// JobScheduler captures job schedule and unschedule methods, and a way to
// introspect the desired state of the scheduling domain.
type JobScheduler interface {
	Schedule(configstore.JobConfig) error
	Unschedule(jobConfigHash string) error
	Snapshot() map[string]configstore.JobConfig
}

// NewHandler returns a http.Handler that serves the API endpoints.
func NewHandler(p Proxy, s JobScheduler) *handler {
	return &handler{
		Proxy:        p,
		JobScheduler: s,
	}
}

// ServeHTTP implements http.Handler.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "PUT" && r.URL.Path == APIVersionPrefix+APISchedulePath:
		h.handleSchedule(w, r)
	case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, APIVersionPrefix+APIUnschedulePath):
		h.handleUnschedule(w, r)
	case r.Method == "GET" && r.URL.Path == APIVersionPrefix+APIProxyPath:
		h.handleProxy(w, r)
	case r.Method == "GET" && r.URL.Path == APIVersionPrefix+APIRegistryPath:
		h.handleRegistry(w, r)
	default:
		http.NotFoundHandler().ServeHTTP(w, r)
	}
}

func (h *handler) handleSchedule(w http.ResponseWriter, r *http.Request) {
	var c configstore.JobConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := c.Valid(); err != nil {
		writeResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.JobScheduler.Schedule(c); err != nil {
		writeResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeResponse(w, http.StatusAccepted, fmt.Sprintf("request to schedule %q (%s) has been accepted", c.Job, c.Hash()))
}

func (h *handler) handleUnschedule(w http.ResponseWriter, r *http.Request) {
	var (
		hash string
		job  string
	)

	if strings.HasSuffix(r.URL.Path, APIUnschedulePath) {
		var c configstore.JobConfig
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			writeResponse(w, http.StatusBadRequest, err.Error())
			return
		}

		if err := c.Valid(); err != nil {
			writeResponse(w, http.StatusBadRequest, err.Error())
			return
		}

		job = c.Job
		hash = c.Hash()
	} else {
		toks := strings.Split(r.URL.Path, "/")

		job = "job"
		hash = toks[len(toks)-1]
	}

	if err := h.JobScheduler.Unschedule(hash); err != nil {
		writeResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeResponse(w, http.StatusAccepted, fmt.Sprintf("request to unschedule %s (%s) has been accepted", job, hash))
}

func (h *handler) handleProxy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(h.Proxy.Snapshot())
}

func (h *handler) handleRegistry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(h.JobScheduler.Snapshot())
}

func writeResponse(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status_code": code,
		"msg":         msg,
	})
}
