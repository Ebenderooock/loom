package searchdebug

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router for the search debug log endpoints.
func Router(store *Store, hub *Hub) chi.Router {
	r := chi.NewRouter()
	r.Get("/", handleList(store))
	r.Get("/active", handleActive(store))
	r.Get("/stats", handleStats(store))
	r.Get("/stream", handleStream(hub))
	r.Get("/{id}", handleGet(store))
	r.Delete("/prune", handlePrune(store))
	return r
}

func handleList(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		offset, _ := strconv.Atoi(q.Get("offset"))
		if limit <= 0 {
			limit = 50
		}

		params := ListParams{
			MediaType: q.Get("media_type"),
			MediaID:   q.Get("media_id"),
			Outcome:   q.Get("outcome"),
			Status:    q.Get("status"),
			Limit:     limit,
			Offset:    offset,
		}

		entries, total, err := store.List(r.Context(), params)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"entries": entries,
			"total":   total,
			"limit":   params.Limit,
			"offset":  params.Offset,
		})
	}
}

func handleActive(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entries, err := store.ListActive(r.Context())
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"entries": entries,
		})
	}
}

func handleGet(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		entry, err := store.Get(r.Context(), id)
		if err != nil {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entry)
	}
}

func handleStats(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := store.Stats(r.Context())
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}
}

func handleStream(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		ch := hub.Subscribe()
		defer hub.Unsubscribe(ch)

		// Send initial keepalive so the client knows we're connected.
		fmt.Fprintf(w, ": connected\n\n")
		flusher.Flush()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case update, ok := <-ch:
				if !ok {
					return
				}
				data, _ := json.Marshal(update)
				fmt.Fprintf(w, "event: search-update\ndata: %s\n\n", data)
				flusher.Flush()
			}
		}
	}
}

func handlePrune(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ageStr := r.URL.Query().Get("max_age")
		maxAge := 7 * 24 * time.Hour // default 7 days
		if ageStr != "" {
			if d, err := time.ParseDuration(ageStr); err == nil {
				maxAge = d
			}
		}

		deleted, err := store.Prune(r.Context(), maxAge)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"deleted": deleted,
		})
	}
}
