package indexers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// mountQueryLog registers the /api/v1/search/log routes.
func (s *Service) mountQueryLog(r chi.Router) {
	if s.queryLog == nil {
		return
	}
	r.Route("/api/v1/search/log", func(r chi.Router) {
		r.Get("/", s.handleQueryLogList)
		r.Delete("/", s.handleQueryLogPrune)
		r.Get("/{id}", s.handleQueryLogGet)
	})
}

func (s *Service) handleQueryLogList(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 50
	}
	entries, err := s.queryLog.ListQueries(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": entries})
}

func (s *Service) handleQueryLogGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	entry, err := s.queryLog.GetQuery(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "query not found")
		} else {
			writeError(w, http.StatusInternalServerError, "get_failed", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (s *Service) handleQueryLogPrune(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		days = 30
	}
	age := time.Duration(days) * 24 * time.Hour
	deleted, err := s.queryLog.PruneOlderThan(r.Context(), age)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "prune_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}
