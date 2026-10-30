package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestRequireFeatureFollowsLiveFlag(t *testing.T) {
	enabled := false
	r := chi.NewRouter()
	r.With(requireFeature(func() bool { return enabled })).Get("/api/v1/x", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	get := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/x", nil))
		return rec
	}

	if rec := get(); rec.Code != http.StatusNotFound || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("disabled: got %d %q, want 404 JSON", rec.Code, rec.Header().Get("Content-Type"))
	}
	enabled = true
	if rec := get(); rec.Code != http.StatusOK {
		t.Fatalf("enabled after toggle: got %d, want 200", rec.Code)
	}
}
