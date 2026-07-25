//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"
)

func TestMoviesFlow(t *testing.T) {
	s := New(t)
	libID := mustCreateLibrary(t, s, t.TempDir())
	qpID := firstQualityProfileID(t, s)

	initial := s.MustGet(t, "/api/v1/movies")
	requireStatus(t, initial, http.StatusOK)
	var initialList struct {
		Total int `json:"total"`
	}
	decodeJSON(t, initial, &initialList)
	if initialList.Total != 0 {
		t.Fatalf("expected empty movies list, got total=%d", initialList.Total)
	}

	create := s.MustPost(t, "/api/v1/movies", map[string]any{
		"title":              "Integration Test Movie",
		"year":               2024,
		"tmdb_id":            "123",
		"quality_profile_id": qpID,
		"library_id":         libID,
		"monitoring_status":  "monitored",
	})
	requireStatus(t, create, http.StatusCreated)
	var created struct {
		ID string `json:"id"`
	}
	decodeJSON(t, create, &created)
	if created.ID == "" {
		t.Fatal("created movie has empty id")
	}

	listAfter := s.MustGet(t, "/api/v1/movies")
	requireStatus(t, listAfter, http.StatusOK)
	var afterList struct {
		Total int `json:"total"`
	}
	decodeJSON(t, listAfter, &afterList)
	if afterList.Total != 1 {
		t.Fatalf("expected total=1 after create, got %d", afterList.Total)
	}

	getOne := s.MustGet(t, "/api/v1/movies/"+created.ID)
	requireStatus(t, getOne, http.StatusOK)
	var one map[string]any
	decodeJSON(t, getOne, &one)
	if one["id"] != created.ID {
		t.Fatalf("expected id=%s, got %v", created.ID, one["id"])
	}

	req, _ := http.NewRequest(http.MethodPut, s.URL+"/api/v1/movies/"+created.ID+"/monitoring", mustJSONBody(t, map[string]any{
		"status": "unmonitored",
	}))
	req.Header.Set("Content-Type", "application/json")
	update, err := s.Client().Do(req)
	if err != nil {
		t.Fatalf("update monitoring: %v", err)
	}
	requireStatus(t, update, http.StatusOK)
	var updated map[string]any
	decodeJSON(t, update, &updated)
	if got := fmt.Sprintf("%v", updated["monitoringStatus"]); got != "unmonitored" {
		t.Fatalf("expected monitoringStatus=unmonitored, got %v", updated["monitoringStatus"])
	}

	delReq, _ := http.NewRequest(http.MethodDelete, s.URL+"/api/v1/movies/"+created.ID, nil)
	delResp, err := s.Client().Do(delReq)
	if err != nil {
		t.Fatalf("delete movie: %v", err)
	}
	requireStatus(t, delResp, http.StatusNoContent)
	_ = delResp.Body.Close()
}
