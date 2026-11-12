package handlers

import (
	"encoding/json"
	"net/http"

	"run_vol/internal/store"
)

func Weeks(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sport := r.URL.Query().Get("sport")
		rows, err := st.WeeklySummary(sport)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if rows == nil {
			rows = []store.WeekRow{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rows)
	}
}
