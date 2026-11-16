package series

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMetadataOnlyRefreshPreservesEpisodeIdentityAndSkipsIngestion(t *testing.T) {
	repo := newMockRepo()
	tmdbID := "42"
	repo.series["show"] = &Series{ID: "show", Title: "Old title", TMDBID: &tmdbID}
	repo.seasons["show"] = []*Season{{ID: "season", SeriesID: "show", SeasonNumber: 1}}
	repo.episodes["show"] = []*Episode{{ID: "imported-episode", SeriesID: "show", SeasonID: "season", HasFile: true}}
	svc := NewService(repo, "synthetic-key", WithPostRefreshHook(func(context.Context, string) {
		t.Fatal("metadata quick action must not invoke ingestion")
	})).(*service)
	svc.httpClient = &http.Client{Transport: airingFixtureTransport(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.Path, "/tv/42") {
			t.Fatalf("unexpected provider request: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"name":"New title","status":"Ended","credits":{"cast":[],"crew":[]}}`))}, nil
	})}
	if err := svc.RefreshSeriesMetadata(context.Background(), "show"); err != nil {
		t.Fatal(err)
	}
	if repo.series["show"].Title != "New title" || repo.series["show"].Status != StatusEnded {
		t.Fatalf("metadata was not refreshed: %+v", repo.series["show"])
	}
	if len(repo.seasons["show"]) != 1 || len(repo.episodes["show"]) != 1 ||
		repo.episodes["show"][0].ID != "imported-episode" || !repo.episodes["show"][0].HasFile {
		t.Fatal("metadata refresh replaced imported episode identity or availability")
	}
}

func TestPostRefreshHook_WiringViaOption(t *testing.T) {
	called := false
	var gotID string
	hook := func(_ context.Context, seriesID string) {
		called = true
		gotID = seriesID
	}

	svc := NewService(newMockRepo(), "", WithPostRefreshHook(hook)).(*service)
	if svc.postRefreshHook == nil {
		t.Fatalf("WithPostRefreshHook did not set the hook")
	}
	svc.postRefreshHook(context.Background(), "abc")
	if !called || gotID != "abc" {
		t.Fatalf("hook not invoked correctly: called=%v id=%q", called, gotID)
	}
}

func TestPostRefreshHook_WiringViaSetter(t *testing.T) {
	called := false
	svc := NewService(newMockRepo(), "").(*service)
	if svc.postRefreshHook != nil {
		t.Fatalf("expected no hook before setter")
	}
	svc.SetPostRefreshHook(func(_ context.Context, _ string) { called = true })
	if svc.postRefreshHook == nil {
		t.Fatalf("SetPostRefreshHook did not set the hook")
	}
	svc.postRefreshHook(context.Background(), "x")
	if !called {
		t.Fatalf("hook set via setter was not invoked")
	}
}
