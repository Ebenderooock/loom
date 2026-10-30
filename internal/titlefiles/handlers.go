package titlefiles

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/ebenderooock/loom/internal/kernel/httpparam"

	"github.com/go-chi/chi/v5"
)

// Router exposes admin-only listing and confirmed deletion endpoints.
// The server supplies its existing administrator/authentication middleware.
func Router(s *Service, admin func(http.Handler) http.Handler) chi.Router {
	r := chi.NewRouter()
	r.Use(admin)
	r.Get("/{mediaType}/{id}", s.handleList)
	r.Post("/{mediaType}/{id}/delete", s.handleDelete)
	return r
}

func requestSeason(r *http.Request) (*int, error) {
	values, present := r.URL.Query()["season"]
	if !present {
		return nil, nil
	}
	if len(values) != 1 {
		return nil, errInvalid
	}
	n, err := strconv.Atoi(values[0])
	if err != nil || n < 0 {
		return nil, errInvalid
	}
	return &n, nil
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	season, err := requestSeason(r)
	if err != nil {
		writeResult(w, r, nil, err)
		return
	}
	listing, err := s.List(r.Context(), chi.URLParam(r, "mediaType"), httpparam.Param(r, "id"), season)
	writeResult(w, r, listing, err)
}

func (s *Service) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Confirm-Delete") != "true" {
		writeResult(w, r, nil, errors.Join(errInvalid, errors.New("X-Confirm-Delete: true is required")))
		return
	}
	season, err := requestSeason(r)
	if err != nil {
		writeResult(w, r, nil, err)
		return
	}
	var body struct {
		Paths        []string `json:"paths"`
		DeletionMode string   `json:"deletionMode"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeResult(w, r, nil, errInvalid)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeResult(w, r, nil, errInvalid)
		return
	}
	deleted, err := s.Delete(r.Context(), chi.URLParam(r, "mediaType"), httpparam.Param(r, "id"), season, body.Paths, body.DeletionMode)
	writeResult(w, r, map[string]any{"deleted": deleted}, err)
}

func writeResult(w http.ResponseWriter, r *http.Request, result any, err error) {
	status := http.StatusOK
	if err != nil {
		message := err.Error()
		switch {
		case errors.Is(err, errInvalid):
			status = http.StatusBadRequest
		case errors.Is(err, errNotFound):
			status = http.StatusNotFound
		case errors.Is(err, errConflict):
			status = http.StatusConflict
		default:
			status = http.StatusInternalServerError
			message = "File operation failed; refresh the file list before retrying."
			slog.ErrorContext(r.Context(), "title file operation failed", "error", err)
		}
		payload, ok := result.(map[string]any)
		if !ok {
			payload = map[string]any{}
		}
		payload["error"] = map[string]string{"message": message}
		result = payload
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(result)
}
