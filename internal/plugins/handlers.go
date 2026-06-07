package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// Tester runs a single plugin synchronously (implemented by *Runner).
type Tester interface {
	RunOnce(ctx context.Context, p *Plugin) *Run
}

// Router mounts plugin endpoints. adminMW guards every route: plugins execute
// arbitrary commands, so the feature is admin-only.
func Router(store *Store, tester Tester, adminMW func(http.Handler) http.Handler) chi.Router {
	r := chi.NewRouter()
	r.Use(adminMW)

	r.Get("/events", listEvents())
	r.Get("/typedefs", getTypeDefs())
	r.Get("/", listPlugins(store))
	r.Post("/", createPlugin(store))
	r.Get("/{id}", getPlugin(store))
	r.Put("/{id}", updatePlugin(store))
	r.Delete("/{id}", deletePlugin(store))
	r.Get("/{id}/runs", listRuns(store))
	r.Post("/{id}/test", testPlugin(store, tester))
	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{
		"code": http.StatusText(status), "message": msg,
	}})
}

func listEvents() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, SupportedEvents)
	}
}

// getTypeDefs serves the ambient .d.ts describing the plugin JS runtime, for the
// editor's IntelliSense.
func getTypeDefs() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"dts": PluginTypeDefs})
	}
}

func listPlugins(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ps, err := store.List(r.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if ps == nil {
			ps = []*Plugin{}
		}
		writeJSON(w, http.StatusOK, ps)
	}
}

func getPlugin(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := store.Get(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, p)
	}
}

func createPlugin(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var p Plugin
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if err := store.Create(r.Context(), &p); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, p)
	}
}

func updatePlugin(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var p Plugin
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		p.ID = chi.URLParam(r, "id")
		if err := store.Update(r.Context(), &p); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, p)
	}
}

func deletePlugin(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func listRuns(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				limit = n
			}
		}
		runs, err := store.ListRuns(r.Context(), chi.URLParam(r, "id"), limit)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if runs == nil {
			runs = []*Run{}
		}
		writeJSON(w, http.StatusOK, runs)
	}
}

func testPlugin(store *Store, tester Tester) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if tester == nil {
			writeErr(w, http.StatusServiceUnavailable, "plugin runner not available")
			return
		}
		p, err := store.Get(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		run := tester.RunOnce(r.Context(), p)
		writeJSON(w, http.StatusOK, run)
	}
}
