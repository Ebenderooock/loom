//go:build integration

package integration

import (
	"net/http"
	"testing"
)

func TestImportListsFlow(t *testing.T) {
	s := New(t)
	libPath := t.TempDir()
	qpID := firstQualityProfileID(t, s)
	_ = mustCreateLibrary(t, s, libPath)

	create := s.MustPost(t, "/api/v1/import-lists", map[string]any{
		"name":                  "TMDB Stub List",
		"list_type":             "tmdb_list",
		"url":                   "123",
		"enabled":               true,
		"media_type":            "movie",
		"library_path":          libPath,
		"quality_profile_id":    qpID,
		"sync_interval_minutes": 60,
		"settings":              "{}",
	})
	requireStatus(t, create, http.StatusCreated)
	var created struct {
		ID string `json:"id"`
	}
	decodeJSON(t, create, &created)
	if created.ID == "" {
		t.Fatal("created import list id is empty")
	}

	list := s.MustGet(t, "/api/v1/import-lists")
	requireStatus(t, list, http.StatusOK)
	var listed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	decodeJSON(t, list, &listed)
	if len(listed.Data) == 0 {
		t.Fatal("expected at least one import list")
	}

	syncResp := s.MustPost(t, "/api/v1/import-lists/"+created.ID+"/sync", map[string]any{})
	requireStatus(t, syncResp, http.StatusOK)
	_ = syncResp.Body.Close()
}
