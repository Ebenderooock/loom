package qualityprofiles

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router for /api/v1/quality-profiles endpoints.
func Router(store *Store, logger *slog.Logger) chi.Router {
	r := chi.NewRouter()
	h := &qpHandler{store: store, logger: logger}
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Get("/{id}", h.get)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.delete)
	r.Get("/{id}/format-scores", h.getFormatScores)
	r.Put("/{id}/format-scores", h.setFormatScores)
	return r
}

type qpHandler struct {
	store  *Store
	logger *slog.Logger
}

func (h *qpHandler) list(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if profiles == nil {
		profiles = []QualityProfile{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": profiles})
}

func (h *qpHandler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	qp, err := h.store.Get(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "quality profile not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, qp)
}

func (h *qpHandler) create(w http.ResponseWriter, r *http.Request) {
	var qp QualityProfile
	if err := json.NewDecoder(r.Body).Decode(&qp); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if qp.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := h.store.Create(r.Context(), &qp); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.logger.Info("quality profile created", "id", qp.ID, "name", qp.Name)
	writeJSON(w, http.StatusCreated, qp)
}

func (h *qpHandler) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var qp QualityProfile
	if err := json.NewDecoder(r.Body).Decode(&qp); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	qp.ID = id
	if err := h.store.Update(r.Context(), &qp); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "quality profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.logger.Info("quality profile updated", "id", qp.ID)
	writeJSON(w, http.StatusOK, qp)
}

func (h *qpHandler) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.Delete(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "quality profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.logger.Info("quality profile deleted", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *qpHandler) getFormatScores(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Verify profile exists first
	if _, err := h.store.Get(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "quality profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items, err := h.store.GetFormatScores(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []FormatItem{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *qpHandler) setFormatScores(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var items []FormatItem
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := h.store.SetFormatScores(r.Context(), id, items); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "quality profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.logger.Info("format scores updated", "profile_id", id, "count", len(items))
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"message": msg},
	})
}
