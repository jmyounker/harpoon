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

func newAPI(s jobScheduler, desired desiredBroadcaster, actual actualBroadcaster) http.Handler {
	r := httprouter.New()

	r.POST("/schedule", handleSchedule(s))
	r.POST("/unschedule", handleUnschedule(s))
	r.POST("/migrate", handleMigrate(s))
	r.GET("/", handleState(desired, actual))

	return r
}

func handleSchedule(s jobScheduler) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		incJobScheduleRequests(1)

		var cfg configstore.JobConfig

		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := s.schedule(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)

		json.NewEncoder(w).Encode(map[string]interface{}{"scheduled": cfg})
	}
}

func handleUnschedule(s jobScheduler) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		incJobUnscheduleRequests(1)

		var cfg configstore.JobConfig

		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := s.unschedule(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)

		json.NewEncoder(w).Encode(map[string]interface{}{"unscheduled": cfg})
	}
}

func handleMigrate(s jobScheduler) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		incJobMigrateRequests(1)

		http.Error(w, "not yet implemented", http.StatusNotImplemented)
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
