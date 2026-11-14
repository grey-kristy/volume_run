package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"volume_run/internal/garmin"
)

func Status(gc *garmin.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"key_set": gc.HasKey()})
	}
}

func SetKey(gc *garmin.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Key string `json:"key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(body.Key) == "" {
			http.Error(w, "key is empty", http.StatusBadRequest)
			return
		}
		gc.SetKey(strings.TrimSpace(body.Key))
		w.WriteHeader(http.StatusNoContent)
	}
}
