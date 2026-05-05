package downloads

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// MountBlocklist attaches /api/v1/blocklist routes to r.
func MountBlocklist(r chi.Router, store *BlocklistStore) {
	r.Route("/api/v1/blocklist", func(r chi.Router) {
		r.Get("/", handleBlocklistList(store))
		r.Delete("/", handleBlocklistClear(store))
		r.Delete("/{id}", handleBlocklistRemove(store))
	})
}

func handleBlocklistList(store *BlocklistStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entries, err := store.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "blocklist_list_failed", err.Error())
			return
		}
		if entries == nil {
			entries = []BlocklistEntry{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": entries})
	}
}

func handleBlocklistRemove(store *BlocklistStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := store.Remove(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, "blocklist_remove_failed", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleBlocklistClear(store *BlocklistStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.Clear(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "blocklist_clear_failed", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
