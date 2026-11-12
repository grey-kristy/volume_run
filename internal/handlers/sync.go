package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"run_vol/internal/garmin"
	"run_vol/internal/store"
)

type syncResponse struct {
	Synced int    `json:"synced"`
	Error  string `json:"error,omitempty"`
}

func Sync(st *store.Store, gc *garmin.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		after, err := st.MaxStartedAt()
		if err != nil {
			log.Printf("sync: max started_at: %v", err)
		}

		activities, err := gc.ListActivities(after)
		if err != nil {
			log.Printf("sync: list activities: %v", err)
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(syncResponse{Error: err.Error()})
			return
		}

		if len(activities) > 0 {
			if err := st.Upsert(activities); err != nil {
				log.Printf("sync: upsert: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(syncResponse{Error: err.Error()})
				return
			}
		}

		json.NewEncoder(w).Encode(syncResponse{Synced: len(activities)})
	}
}
