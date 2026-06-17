package systemlogs

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/kernel/logging"
)

// HandlerDeps groups the dependencies for the system logs HTTP handlers.
type HandlerDeps struct {
	Store   *Store
	Buffer  *logging.RingBuffer
	Capture *logging.CaptureHandler
}

// Router returns a chi router for system log endpoints.
func Router(deps HandlerDeps, adminOnly func(http.Handler) http.Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/", handleList(deps.Store))
	r.Get("/stream", handleStream(deps.Buffer))
	r.Get("/config", handleGetConfig(deps.Capture))
	if adminOnly != nil {
		r.With(adminOnly).Put("/config", handleUpdateConfig(deps.Capture))
		r.With(adminOnly).Delete("/", handleClear(deps.Store))
	} else {
		r.Put("/config", handleUpdateConfig(deps.Capture))
		r.Delete("/", handleClear(deps.Store))
	}
	return r
}

func handleList(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		offset, _ := strconv.Atoi(q.Get("offset"))
		if limit <= 0 {
			limit = 100
		}

		f := ListFilter{
			Level:      q.Get("level"),
			Search:     q.Get("search"),
			WorkflowID: q.Get("workflow_id"),
			Since:      q.Get("since"),
			Until:      q.Get("until"),
			Limit:      limit,
			Offset:     offset,
		}

		result, err := store.List(r.Context(), f)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func handleStream(buffer *logging.RingBuffer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "streaming not supported")
			return
		}

		workflowID := r.URL.Query().Get("workflow_id")

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		flusher.Flush()

		subID, ch := buffer.Subscribe(256)
		defer buffer.Unsubscribe(subID)

		for {
			select {
			case <-r.Context().Done():
				return
			case entry, ok := <-ch:
				if !ok {
					return
				}
				if workflowID != "" && entry.WorkflowID != workflowID {
					continue
				}
				data, err := json.Marshal(entry)
				if err != nil {
					continue
				}
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	}
}

type logConfigResponse struct {
	CaptureLevel string `json:"capture_level"`
}

func handleGetConfig(capture *logging.CaptureHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		level := capture.GetCaptureLevel()
		writeJSON(w, http.StatusOK, logConfigResponse{
			CaptureLevel: levelToString(level),
		})
	}
}

type updateConfigRequest struct {
	CaptureLevel string `json:"capture_level"`
}

func handleUpdateConfig(capture *logging.CaptureHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if req.CaptureLevel != "" {
			level, err := logging.ParseCaptureLevel(req.CaptureLevel)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			capture.SetCaptureLevel(level)
		}
		writeJSON(w, http.StatusOK, logConfigResponse{
			CaptureLevel: levelToString(capture.GetCaptureLevel()),
		})
	}
}

func handleClear(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.Clear(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func levelToString(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "error"
	case l >= slog.LevelWarn:
		return "warn"
	case l >= slog.LevelInfo:
		return "info"
	default:
		return "debug"
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func levelFromString(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelWarn
	}
}
