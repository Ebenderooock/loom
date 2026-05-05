package indexers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// mountSearchHealth registers the search-health dashboard routes.
func (s *Service) mountSearchHealth(r chi.Router) {
	r.Route("/api/v1/indexers/health", func(r chi.Router) {
		r.Get("/", s.handleSearchHealthList)
		r.Post("/reset", s.handleSearchHealthResetAll)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", s.handleSearchHealthGet)
			r.Post("/reset", s.handleSearchHealthReset)
		})
	})
}

func (s *Service) handleSearchHealthList(w http.ResponseWriter, r *http.Request) {
	all := s.searchHealthTracker.GetAllHealth()
	writeJSON(w, http.StatusOK, map[string]any{"data": all})
}

func (s *Service) handleSearchHealthGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Check whether this indexer exists at all: either tracked or registered.
	_, inRegistry := s.registry.Get(id)
	h := s.searchHealthTracker.GetHealth(id)
	if h.TotalSearches == 0 && !inRegistry {
		writeError(w, http.StatusNotFound, "not_found", "indexer not found")
		return
	}
	writeJSON(w, http.StatusOK, h)
}

func (s *Service) handleSearchHealthResetAll(w http.ResponseWriter, _ *http.Request) {
	s.searchHealthTracker.ResetAll()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Service) handleSearchHealthReset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.searchHealthTracker.Reset(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
