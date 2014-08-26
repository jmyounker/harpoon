package main

import (
	"encoding/json"
	"net/http"

	"github.com/julienschmidt/httprouter"

	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

type api struct {
	http.Handler
	jobScheduler
}

func newAPI(target registry, actual actualBroadcaster) http.Handler {
	r := httprouter.New()

	r.POST("/schedule", handleSchedule(target))
	r.POST("/unschedule", handleUnschedule(target))
	r.POST("/migrate", handleMigrate(target))
	r.GET("/", handleState(target, actual))

	return r
}

func handleSchedule(s jobScheduler) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var cfg configstore.JobConfig

		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := cfg.Valid(); err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := s.schedule(cfg); err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)

		json.NewEncoder(w).Encode(map[string]interface{}{"scheduled": cfg})
	}
}

func handleUnschedule(s jobScheduler) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var cfg configstore.JobConfig

		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := cfg.Valid(); err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := s.unschedule(cfg); err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)

		json.NewEncoder(w).Encode(map[string]interface{}{"unscheduled": cfg})
	}
}

func handleMigrate(s jobScheduler) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		incJobMigrateRequests(1)

		writeError(w, "not yet implemented", http.StatusNotImplemented)
	}
}

func handleState(desired desiredBroadcaster, actual actualBroadcaster) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"desired": desired.snapshot(),
			"actual":  actual.snapshot(),
		})
	}
}

func writeError(w http.ResponseWriter, err string, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status_code": code,
		"error":       err,
	})
}
