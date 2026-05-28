package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"quantsim-server/db"
)

// HistoryHandler serves GET /api/history?limit=N
// Returns the most recent N ticks as a JSON array, oldest-first.
func HistoryHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 200
		if s := r.URL.Query().Get("limit"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				limit = n
			}
		}
		if limit > 1000 {
			limit = 1000
		}

		ticks, err := store.History(limit)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if ticks == nil {
			ticks = []json.RawMessage{}
		}

		data, err := json.Marshal(ticks)
		if err != nil {
			http.Error(w, "marshal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(data)
	}
}
