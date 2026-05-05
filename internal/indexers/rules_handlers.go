package indexers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// MountRuleRoutes attaches indexer rule endpoints.
func MountRuleRoutes(r chi.Router, ruleStore *RuleStore, svc *Service) {
	r.Route("/api/v1/indexers/rules", func(r chi.Router) {
		r.Get("/", handleListRules(ruleStore))
		r.Post("/", handleCreateRule(ruleStore))
		r.Delete("/{ruleID}", handleDeleteRule(ruleStore))
	})
	r.Post("/api/v1/indexers/import-jackett", handleImportJackett(svc))
}

func handleListRules(store *RuleStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rules, err := store.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list_rules_failed", err.Error())
			return
		}
		if rules == nil {
			rules = []IndexerRule{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": rules})
	}
}

func handleCreateRule(store *RuleStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var rule IndexerRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
		if rule.IndexerID == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "indexer_id is required")
			return
		}
		if rule.CategoryFilter == nil {
			rule.CategoryFilter = []int{}
		}
		if rule.TagFilter == nil {
			rule.TagFilter = []string{}
		}
		rule.Enabled = true

		if err := store.Create(r.Context(), &rule); err != nil {
			writeError(w, http.StatusInternalServerError, "create_rule_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"data": rule})
	}
}

func handleDeleteRule(store *RuleStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "ruleID")
		if err := store.Delete(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, "delete_rule_failed", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleImportJackett(svc *Service) http.HandlerFunc {
	type request struct {
		JackettURL string `json:"jackett_url"`
		APIKey     string `json:"api_key"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
			return
		}
		if req.JackettURL == "" || req.APIKey == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "jackett_url and api_key are required")
			return
		}

		result, err := ImportFromJackett(r.Context(), req.JackettURL, req.APIKey, svc)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "import_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": result})
	}
}
