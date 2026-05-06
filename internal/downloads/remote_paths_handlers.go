package downloads

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// MountRemotePathRoutes returns a RouteMounter that adds remote-path-mapping
// CRUD endpoints under /api/v1/download-clients/remote-path-mappings.
func MountRemotePathRoutes(store *RemotePathStore) RouteMounter {
	return func(r chi.Router) {
		r.Route("/api/v1/download-clients/remote-path-mappings", func(r chi.Router) {
			r.Get("/", handleListMappings(store))
			r.Post("/", handleCreateMapping(store))
			r.Delete("/{id}", handleDeleteMapping(store))
		})
	}
}

func handleListMappings(store *RemotePathStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mappings, err := store.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list_failed", err.Error())
			return
		}
		if mappings == nil {
			mappings = []RemotePathMapping{}
		}
		writeJSON(w, http.StatusOK, mappings)
	}
}

func handleCreateMapping(store *RemotePathStore) http.HandlerFunc {
	type createBody struct {
		ClientID   string `json:"client_id"`
		RemotePath string `json:"remote_path"`
		LocalPath  string `json:"local_path"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body createBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
		if body.ClientID == "" || body.RemotePath == "" || body.LocalPath == "" {
			writeError(w, http.StatusBadRequest, "missing_fields", "client_id, remote_path, and local_path are required")
			return
		}

		m, err := store.Create(r.Context(), RemotePathMapping{
			ClientID:   body.ClientID,
			RemotePath: body.RemotePath,
			LocalPath:  body.LocalPath,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, m)
	}
}

func handleDeleteMapping(store *RemotePathStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "missing_id", "mapping id is required")
			return
		}
		if err := store.Delete(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, "delete_failed", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
