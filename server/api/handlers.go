package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"quantsim-server/db"
	"quantsim-server/sandbox"
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

// TraderHandler routes /api/traders and /api/traders/<id>[/log] requests.
func TraderHandler(mgr *sandbox.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		suffix := strings.TrimPrefix(r.URL.Path, "/api/traders")
		// suffix is "" | "/" | "/<id>" | "/<id>/log"

		// Collection endpoints: "" or "/"
		if suffix == "" || suffix == "/" {
			switch r.Method {
			case http.MethodGet:
				list := mgr.List()
				if list == nil {
					list = []sandbox.SandboxInfo{}
				}
				json.NewEncoder(w).Encode(list)

			case http.MethodPost:
				var req struct {
					TraderID string `json:"trader_id"`
					Script   string `json:"script"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]string{"error": "invalid_json"})
					return
				}
				if err := mgr.Spawn(req.TraderID, req.Script); err != nil {
					msg := err.Error()
					switch msg {
					case "invalid_trader_id", "script_too_large", "too_many_sandboxes", "duplicate_trader_id":
						w.WriteHeader(http.StatusBadRequest)
					default:
						w.WriteHeader(http.StatusInternalServerError)
					}
					json.NewEncoder(w).Encode(map[string]string{"error": msg})
					return
				}
				json.NewEncoder(w).Encode(map[string]string{
					"trader_id": req.TraderID,
					"status":    "spawning",
				})

			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
			return
		}

		// Individual endpoints: "/<id>" or "/<id>/log"
		parts := strings.SplitN(strings.TrimPrefix(suffix, "/"), "/", 2)
		traderID := parts[0]
		subpath := ""
		if len(parts) == 2 {
			subpath = parts[1]
		}

		if subpath == "log" && r.Method == http.MethodGet {
			logs, err := mgr.ContainerLogs(traderID, "100")
			if err != nil {
				if err.Error() == "not_found" {
					w.WriteHeader(http.StatusNotFound)
					json.NewEncoder(w).Encode(map[string]string{"error": "not_found"})
				} else {
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				}
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"log": logs})
			return
		}

		switch r.Method {
		case http.MethodGet:
			info, ok := mgr.Get(traderID)
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]string{"error": "not_found"})
				return
			}
			json.NewEncoder(w).Encode(info)

		case http.MethodDelete:
			if err := mgr.Kill(traderID); err != nil {
				if err.Error() == "not_found" {
					w.WriteHeader(http.StatusNotFound)
					json.NewEncoder(w).Encode(map[string]string{"error": "not_found"})
				} else {
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				}
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}
