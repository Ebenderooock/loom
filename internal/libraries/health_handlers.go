package libraries

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/auth"
)

type healthHandlers struct {
	store   *Store
	scanner *Scanner
	logger  *slog.Logger
	mu      sync.Mutex
	active  map[string]bool
	slots   chan struct{}
}

func mountHealth(r chi.Router, store *Store, scanner *Scanner, logger *slog.Logger) {
	h := &healthHandlers{store: store, scanner: scanner, logger: logger,
		active: make(map[string]bool), slots: make(chan struct{}, 2)}
	r.With(healthAdmin).Get("/{id}/health", h.get)
	r.With(healthAdmin).Post("/{id}/health", h.run)
	r.With(healthAdmin).Post("/{id}/health/dismiss", h.dismiss)
}

func healthAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := auth.IdentityFrom(r.Context()); id != nil && !id.HasRole("admin") {
			writeError(w, http.StatusForbidden, "admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *healthHandlers) library(w http.ResponseWriter, r *http.Request) *Library {
	lib, err := h.store.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			h.fail(w, err)
		}
		return nil
	}
	return lib
}

func (h *healthHandlers) fail(w http.ResponseWriter, err error) {
	h.logger.Error("library health", "error", err)
	writeError(w, http.StatusInternalServerError, "library health operation failed")
}

func (h *healthHandlers) get(w http.ResponseWriter, r *http.Request) {
	lib := h.library(w, r)
	if lib == nil {
		return
	}
	report, err := h.store.loadHealth(r.Context(), lib.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *healthHandlers) acquire(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active[id] {
		return false
	}
	select {
	case h.slots <- struct{}{}:
		h.active[id] = true
		return true
	default:
		return false
	}
}

func (h *healthHandlers) release(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.active, id)
	<-h.slots
}

func (h *healthHandlers) run(w http.ResponseWriter, r *http.Request) {
	lib := h.library(w, r)
	if lib == nil {
		return
	}
	if !h.acquire(lib.ID) {
		writeError(w, http.StatusConflict, "health check already active or concurrency limit reached; retry later")
		return
	}
	defer h.release(lib.ID)
	if h.scanner != nil {
		if !h.scanner.scanMu.TryRLock() {
			writeError(w, http.StatusConflict, "library scan active; wait for it to finish before checking health")
			return
		}
		defer h.scanner.scanMu.RUnlock()
	}
	ctx, cancel := context.WithTimeout(r.Context(), healthTimeout)
	defer cancel()
	report, err := h.store.checkHealth(ctx, lib, healthEntryLimit)
	if err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		report = newHealthCheck(lib, healthEntryLimit).report
		report.Status = "cancelled"
		report.Warnings = append(report.Warnings, "Cancelled before records could be collected; no absence conclusions are available.")
		err = nil
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	// Preserve a cancelled partial snapshot after the client disconnects, but do
	// not detach the actual scan from request cancellation.
	saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
	defer saveCancel()
	if err := h.store.applyHealthDismissals(saveCtx, report); err != nil {
		h.fail(w, err)
		return
	}
	if err := h.store.saveHealth(saveCtx, report); err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *healthHandlers) dismiss(w http.ResponseWriter, r *http.Request) {
	lib := h.library(w, r)
	if lib == nil {
		return
	}
	var req struct {
		FindingID string `json:"finding_id"`
		Dismissed *bool  `json:"dismissed"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || req.FindingID == "" || req.Dismissed == nil {
		writeError(w, http.StatusBadRequest, "finding_id and dismissed are required")
		return
	}
	report, err := h.store.loadHealth(r.Context(), lib.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	found := false
	if report != nil {
		for _, finding := range report.Findings {
			if finding.ID == req.FindingID {
				found = true
				break
			}
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "finding not present in latest report; recheck")
		return
	}
	if *req.Dismissed {
		_, err = h.store.db.ExecContext(r.Context(), `INSERT INTO library_health_dismissals (library_id, finding_id)
			VALUES (?, ?) ON CONFLICT(library_id, finding_id) DO NOTHING`, lib.ID, req.FindingID)
	} else {
		_, err = h.store.db.ExecContext(r.Context(), `DELETE FROM library_health_dismissals WHERE library_id = ? AND finding_id = ?`, lib.ID, req.FindingID)
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	if err := h.store.applyHealthDismissals(r.Context(), report); err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
