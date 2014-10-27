// Package api implements REST-y handlers for interacting with the scheduler.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
	"github.com/soundcloud/harpoon/harpoon-scheduler/registry"
	"github.com/soundcloud/harpoon/harpoon-scheduler/reprproxy"
)

type handler struct {
	reprproxy.Proxy
	*registry.Registry
}

// NewHandler returns a http.Handler that serves the API endpoints.
func NewHandler(p reprproxy.Proxy, r *registry.Registry) *handler {
	return &handler{
		Proxy:    p,
		Registry: r,
	}
}

// ServeHTTP implements http.Handler.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "PUT" && r.URL.Path == "/api/v0/schedule":
		h.handleSchedule(w, r)
	case r.Method == "PUT" && r.URL.Path == "/api/v0/unschedule":
		h.handleUnschedule(w, r)
	case r.Method == "GET" && r.URL.Path == "/api/v0/proxy":
		h.handleProxy(w, r)
	case r.Method == "GET" && r.URL.Path == "/api/v0/registry":
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

	if err := h.Registry.Schedule(c); err != nil {
		writeResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeResponse(w, http.StatusAccepted, fmt.Sprintf("request to schedule %q has been accepted", c.Job))
}

func (h *handler) handleUnschedule(w http.ResponseWriter, r *http.Request) {
	var c configstore.JobConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.Registry.Unschedule(c); err != nil {
		writeResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeResponse(w, http.StatusAccepted, fmt.Sprintf("request to schedule %q has been accepted", c.Job))
}

func (h *handler) handleProxy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(h.Proxy.Snapshot())
}

func (h *handler) handleRegistry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(h.Registry.Snapshot())
}

func writeResponse(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status_code": code,
		"msg":         msg,
	})
}
