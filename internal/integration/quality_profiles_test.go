//go:build integration

package integration

import (
	"net/http"
	"testing"
)

func TestQualityProfilesFlow(t *testing.T) {
	s := New(t)

	list := s.MustGet(t, "/api/v1/quality-profiles")
	requireStatus(t, list, http.StatusOK)
	var payload struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	decodeJSON(t, list, &payload)
	if len(payload.Data) == 0 {
		t.Fatal("expected seeded quality profiles")
	}

	create := s.MustPost(t, "/api/v1/quality-profiles", map[string]any{
		"name":                "Custom Integration Profile",
		"cutoff":              "",
		"items":               "[]",
		"upgrade_allowed":     true,
		"min_format_score":    0,
		"cutoff_format_score": 0,
	})
	requireStatus(t, create, http.StatusCreated)
	var created struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	decodeJSON(t, create, &created)
	if created.ID == "" || created.Name == "" {
		t.Fatalf("unexpected create response: %+v", created)
	}
}
