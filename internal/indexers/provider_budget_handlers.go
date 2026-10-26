package indexers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/auditlog"
)

func (s *Service) handleProviderBudgets(w http.ResponseWriter, r *http.Request) {
	if !requireAdminIfPresent(w, r) {
		return
	}
	if s.providerBudgets == nil {
		writeError(w, http.StatusServiceUnavailable, "budgets_unavailable", "provider budgets not configured")
		return
	}
	budgets, err := s.providerBudgets.List(r.Context())
	if err != nil {
		s.logger.Error("list provider budgets failed", "error", err)
		writeError(w, http.StatusInternalServerError, "budgets_failed", "cannot read provider budgets")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"budgets": budgets})
}

func (s *Service) handleConfigureProviderBudget(w http.ResponseWriter, r *http.Request) {
	if !requireAdminIfPresent(w, r) {
		return
	}
	if s.providerBudgets == nil {
		writeError(w, http.StatusServiceUnavailable, "budgets_unavailable", "provider budgets not configured")
		return
	}
	var req struct {
		Allowance     int64    `json:"allowance"`
		WindowSeconds int64    `json:"window_seconds"`
		IndexerIDs    []string `json:"indexer_ids"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid budget configuration")
		return
	}
	if req.IndexerIDs == nil {
		writeError(w, http.StatusBadRequest, "invalid_budget", "indexer_ids must be an explicit list")
		return
	}
	b := ProviderBudget{ID: chi.URLParam(r, "budgetID"), Allowance: req.Allowance,
		WindowSeconds: req.WindowSeconds, IndexerIDs: req.IndexerIDs}
	if err := s.providerBudgets.Configure(r.Context(), b); err != nil {
		s.logger.Warn("configure provider budget failed", "error", err)
		writeError(w, http.StatusBadRequest, "invalid_budget", "invalid budget label, allowance, window, or membership")
		return
	}
	s.auditLog.Log(r.Context(), auditlog.Entry{
		Category: "indexer", EventType: "indexer.budget_configured",
		Message: "Provider budget configuration changed; accounting retained",
		Level:   "warn", Source: auditlog.StrPtr("user"),
	})
	w.WriteHeader(http.StatusNoContent)
}
