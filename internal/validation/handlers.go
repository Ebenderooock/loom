package validation

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Router returns a chi.Router for validation rule management.
func Router(v *Validator) chi.Router {
	r := chi.NewRouter()
	r.Get("/rules", handleGetRules(v))
	r.Put("/rules", handleUpdateRules(v))
	r.Post("/check", handleValidateFile(v))
	return r
}

func handleGetRules(v *Validator) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		rules := v.Rules()
		if rules == nil {
			rules = []ValidationRule{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": rules})
	}
}

func handleUpdateRules(v *Validator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var rules []ValidationRule
		if err := json.NewDecoder(r.Body).Decode(&rules); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		v.SetRules(rules)
		writeJSON(w, http.StatusOK, map[string]any{"data": v.Rules()})
	}
}

func handleValidateFile(v *Validator) http.HandlerFunc {
	type request struct {
		Path string `json:"path"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if req.Path == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
			return
		}
		result := v.Validate(req.Path)
		writeJSON(w, http.StatusOK, map[string]any{"data": result})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
