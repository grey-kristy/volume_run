package handlers

import (
	"encoding/json"
	"net/http"

	"run_vol/internal/store"
)

func Activities(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		week := r.URL.Query().Get("week")
		if week == "" {
			http.Error(w, "missing week parameter", http.StatusBadRequest)
			return
		}
		sport := r.URL.Query().Get("sport")

		rows, err := st.WeekActivities(week, sport)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if rows == nil {
			rows = []store.ActivityRow{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rows)
	}
}
