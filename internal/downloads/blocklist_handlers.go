package downloads

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// MountBlocklist attaches /api/v1/blocklist routes to r.
func MountBlocklist(r chi.Router, store *BlocklistStore) {
	r.Route("/api/v1/blocklist", func(r chi.Router) {
		r.Get("/", handleBlocklistList(store))
		r.Post("/check", handleBlocklistCheck(store))
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

type blocklistCheckRelease struct {
	Title    string `json:"title"`
	Infohash string `json:"infohash,omitempty"`
	Magnet   string `json:"magnet,omitempty"`
}

// maxBlocklistCheck bounds a single check request.
const maxBlocklistCheck = 2000

// handleBlocklistCheck reports, per release, whether it is blocklisted.
// The response's "blocked" slice is index-aligned with the request.
func handleBlocklistCheck(store *BlocklistStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Releases []blocklistCheckRelease `json:"releases"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
		if len(req.Releases) > maxBlocklistCheck {
			writeError(w, http.StatusBadRequest, "too_many_releases", "too many releases in one request")
			return
		}
		blocked := make([]bool, len(req.Releases))
		for i, rel := range req.Releases {
			hash := NormalizeInfoHash(rel.Infohash)
			if hash == "" {
				hash = InfoHashFromMagnet(rel.Magnet)
			}
			ok, err := store.IsReleaseBlocked(r.Context(), rel.Title, hash)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "blocklist_check_failed", err.Error())
				return
			}
			blocked[i] = ok
		}
		writeJSON(w, http.StatusOK, map[string]any{"blocked": blocked})
	}
}
