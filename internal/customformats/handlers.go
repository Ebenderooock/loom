package customformats

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/qualityprofiles"
)

// Router returns a chi.Router for the /api/v1/custom-formats endpoints.
func Router(store *Store, logger *slog.Logger) chi.Router {
	r := chi.NewRouter()
	h := &handler{store: store, logger: logger}
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Post("/test", h.test)
	r.Get("/tiers", h.listTiers)
	r.Post("/tiers", h.createTier)
	r.Put("/tiers/{id}", h.updateTier)
	r.Delete("/tiers/{id}", h.deleteTier)
	r.Get("/{id}", h.get)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.delete)
	return r
}

func (h *handler) listTiers(w http.ResponseWriter, r *http.Request) {
	tiers, err := h.store.ListTiers(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tiers == nil {
		tiers = []Tier{}
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": tiers})
}

func (h *handler) createTier(w http.ResponseWriter, r *http.Request) {
	h.saveTier(w, r, true)
}

func (h *handler) updateTier(w http.ResponseWriter, r *http.Request) {
	h.saveTier(w, r, false)
}

func (h *handler) saveTier(w http.ResponseWriter, r *http.Request, create bool) {
	var tier Tier
	if err := json.NewDecoder(r.Body).Decode(&tier); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if create {
		// Use a temporary ID solely for validation; the store assigns the real ID.
		tier.ID = "new"
	} else {
		tier.ID = chi.URLParam(r, "id")
	}
	if err := tier.Validate(); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.SaveTier(r.Context(), &tier, create); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.writeError(w, http.StatusNotFound, "custom tier not found")
		} else {
			h.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	status := http.StatusOK
	if create {
		status = http.StatusCreated
	}
	h.writeJSON(w, status, tier)
}

func (h *handler) deleteTier(w http.ResponseWriter, r *http.Request) {
	err := h.store.DeleteTier(r.Context(), chi.URLParam(r, "id"))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		h.writeError(w, http.StatusNotFound, "custom tier not found")
	case errors.Is(err, ErrTierReferenced):
		h.writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		h.writeError(w, http.StatusInternalServerError, err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

type handler struct {
	store  *Store
	logger *slog.Logger
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	formats, err := h.store.List(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if formats == nil {
		formats = []CustomFormat{}
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": formats})
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cf, err := h.store.Get(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		h.writeError(w, http.StatusNotFound, "custom format not found")
		return
	}
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, cf)
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var cf CustomFormat
	if err := json.NewDecoder(r.Body).Decode(&cf); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if cf.ID == "" || cf.Name == "" {
		h.writeError(w, http.StatusBadRequest, "id and name are required")
		return
	}
	if err := h.store.Create(r.Context(), &cf); err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.logger.InfoContext(r.Context(), "custom format created", "id", cf.ID, "name", cf.Name)
	h.writeJSON(w, http.StatusCreated, cf)
}

func (h *handler) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var cf CustomFormat
	if err := json.NewDecoder(r.Body).Decode(&cf); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	cf.ID = id
	if err := h.store.Update(r.Context(), &cf); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.writeError(w, http.StatusNotFound, "custom format not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.logger.InfoContext(r.Context(), "custom format updated", "id", cf.ID)
	h.writeJSON(w, http.StatusOK, cf)
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.Delete(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.writeError(w, http.StatusNotFound, "custom format not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.logger.InfoContext(r.Context(), "custom format deleted", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// test evaluates a release title against all stored custom formats.
func (h *handler) test(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title            string `json:"title"`
		QualityProfileID string `json:"quality_profile_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Title == "" {
		h.writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	formats, err := h.store.List(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	ri := ParseReleaseName(req.Title)
	engine := NewEngine(formats)
	matches := engine.ScoreRelease(ri)
	if matches == nil {
		matches = []FormatMatch{}
	}

	response := map[string]any{
		"release": ri,
		"matches": matches,
		"score":   TotalScore(matches),
	}
	if req.QualityProfileID != "" {
		profile, err := qualityprofiles.NewStore(h.store.db).Get(r.Context(), req.QualityProfileID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				h.writeError(w, http.StatusNotFound, "quality profile not found")
			} else {
				h.writeError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		scores := make(map[string]int, len(profile.FormatItems))
		for _, item := range profile.FormatItems {
			scores[item.FormatID] = item.Score
		}
		total := ApplyScores(matches, scores)
		response["score"] = total
		response["quality_profile_id"] = profile.ID
		response["minimum_score"] = profile.MinFormatScore
		response["meets_minimum"] = total >= profile.MinFormatScore
		if HasCustomTiers(profile) {
			tiers, err := h.store.ListTiers(r.Context())
			if err != nil {
				h.writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			tier, err := ResolveTier(profile, tiers, ri, BaseTierID(profile, ri.Quality))
			if err != nil {
				h.writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			response["quality_tier"] = tier
		}
	}
	h.writeJSON(w, http.StatusOK, response)
}

func (h *handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *handler) writeError(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, map[string]any{
		"error": map[string]string{"message": msg},
	})
}
