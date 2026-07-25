//go:build integration

package integration

import (
	"net/http"
	"testing"
)

func TestHealth(t *testing.T) {
	s := New(t)
	resp, err := http.Get(s.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	requireStatus(t, resp, http.StatusOK)
	var body struct {
		Status string `json:"status"`
	}
	decodeJSON(t, resp, &body)
	if body.Status != "ok" {
		t.Fatalf("expected status=ok, got %q", body.Status)
	}
}
