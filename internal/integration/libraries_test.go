//go:build integration

package integration

import (
	"net/http"
	"testing"
)

func TestLibrariesFlow(t *testing.T) {
	s := New(t)
	libPath := t.TempDir()

	create := s.MustPost(t, "/api/v1/libraries", map[string]any{
		"name":       "Library Integration",
		"path":       libPath,
		"media_type": "movie",
	})
	requireStatus(t, create, http.StatusCreated)
	var created struct {
		ID string `json:"id"`
	}
	decodeJSON(t, create, &created)
	if created.ID == "" {
		t.Fatal("created library id is empty")
	}

	list := s.MustGet(t, "/api/v1/libraries")
	requireStatus(t, list, http.StatusOK)
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	decodeJSON(t, list, &payload)
	if len(payload.Data) != 1 {
		t.Fatalf("expected one library, got %d", len(payload.Data))
	}

	delReq, _ := http.NewRequest(http.MethodDelete, s.URL+"/api/v1/libraries/"+created.ID, nil)
	delResp, err := s.Client().Do(delReq)
	if err != nil {
		t.Fatalf("delete library: %v", err)
	}
	requireStatus(t, delResp, http.StatusNoContent)
	_ = delResp.Body.Close()
}
