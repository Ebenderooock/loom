package connect

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router with all connect endpoints mounted.
func Router(service Service) chi.Router {
	r := chi.NewRouter()

	r.Get("/", listConnections(service))
	r.Post("/", createConnection(service))
	r.Post("/test", testConnectionConfig(service))

	r.Mount("/trakt/oauth", TraktOAuthRouter(service))
	r.Mount("/trakt/sync", TraktSyncRouter(service))

	r.Get("/{id}", getConnection(service))
	r.Put("/{id}", updateConnection(service))
	r.Delete("/{id}", deleteConnection(service))
	r.Post("/{id}/test", testConnectionByID(service))

	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    http.StatusText(status),
			"message": msg,
		},
	})
}

func listConnections(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conns, err := svc.ListConnections(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if conns == nil {
			conns = []*Connection{}
		}
		writeJSON(w, http.StatusOK, conns)
	}
}

func createConnection(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.Provider == "" {
			writeError(w, http.StatusBadRequest, "provider is required")
			return
		}

		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		notifyOnImport := true
		if req.NotifyOnImport != nil {
			notifyOnImport = *req.NotifyOnImport
		}

		c := &Connection{
			Name:           req.Name,
			Provider:       req.Provider,
			Enabled:        enabled,
			Settings:       req.Settings,
			NotifyOnImport: notifyOnImport,
		}

		if err := svc.CreateConnection(r.Context(), c); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, c)
	}
}

func getConnection(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		conn, err := svc.GetConnection(r.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, conn)
	}
}

func updateConnection(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		existing, err := svc.GetConnection(r.Context(), id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		var req UpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		if req.Name != nil {
			existing.Name = *req.Name
		}
		if req.Provider != nil {
			existing.Provider = *req.Provider
		}
		if req.Enabled != nil {
			existing.Enabled = *req.Enabled
		}
		if req.Settings != nil {
			existing.Settings = *req.Settings
		}
		if req.NotifyOnImport != nil {
			existing.NotifyOnImport = *req.NotifyOnImport
		}

		if err := svc.UpdateConnection(r.Context(), existing); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, existing)
	}
}

func deleteConnection(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.DeleteConnection(r.Context(), id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func testConnectionByID(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.TestConnection(r.Context(), id); err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"message": "connection test successful"})
	}
}

func testConnectionConfig(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if req.Provider == "" {
			writeError(w, http.StatusBadRequest, "provider is required")
			return
		}

		if err := svc.TestConnectionConfig(r.Context(), req.Provider, req.Settings); err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"message": "connection test successful"})
	}
}
