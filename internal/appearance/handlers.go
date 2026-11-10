package appearance

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

const maxBodyBytes = 16 * 1024

// Router must be mounted inside the authenticated router. adminMW protects
// writes; nil follows the server's auth-disabled convention.
func Router(store *Store, logger *slog.Logger, adminMW func(http.Handler) http.Handler) chi.Router {
	if logger == nil {
		logger = slog.Default()
	}
	if adminMW == nil {
		adminMW = func(next http.Handler) http.Handler { return next }
	}
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		settings, err := store.Get(r.Context())
		if err != nil {
			logger.Error("appearance: failed to read settings", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read appearance settings"})
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})
	r.With(adminMW).Put("/", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		// Pointers distinguish omitted/null fields from valid zero values. PUT
		// replaces the full object, never merges a partial update.
		var body struct {
			Theme         *string `json:"theme"`
			Accent        *string `json:"accent"`
			BackgroundURL *string `json:"backgroundUrl"`
			BackgroundDim *int    `json:"backgroundDim"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		err := decoder.Decode(&body)
		if err == nil {
			var extra any
			err = decoder.Decode(&extra)
			if errors.Is(err, io.EOF) {
				err = nil
			} else if err == nil {
				err = errors.New("multiple JSON values")
			}
		}
		if err != nil {
			status := http.StatusBadRequest
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				status = http.StatusRequestEntityTooLarge
			}
			writeJSON(w, status, map[string]string{"error": "invalid appearance body"})
			return
		}
		if body.Theme == nil || body.Accent == nil || body.BackgroundURL == nil || body.BackgroundDim == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "all appearance fields are required"})
			return
		}
		settings := Settings{Theme: *body.Theme, Accent: *body.Accent, BackgroundURL: *body.BackgroundURL, BackgroundDim: *body.BackgroundDim}
		if err := settings.Validate(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := store.Set(r.Context(), settings); err != nil {
			logger.Error("appearance: failed to write settings", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save appearance settings"})
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})
	return r
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
