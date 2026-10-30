package collections

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ebenderooock/loom/internal/metadata/tmdb"
	"github.com/ebenderooock/loom/internal/metadata/tvdb"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/series"
)

func TestBackgroundScanPaginatesResumesAndDoesNotStarveShows(t *testing.T) {
	ctx, store := context.Background(), testStore(t)
	var movieCalls, showCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/login":
			_, _ = w.Write([]byte(`{"data":{"token":"test"}}`))
		case strings.HasPrefix(r.URL.Path, "/movie/"):
			movieCalls.Add(1)
			id := strings.TrimPrefix(r.URL.Path, "/movie/")
			_, _ = fmt.Fprintf(w, `{"id":%s,"title":"Movie %s"}`, id, id)
		case strings.HasPrefix(r.URL.Path, "/series/"):
			showCalls.Add(1)
			_, _ = w.Write([]byte(`{"status":"success","data":{"id":1,"name":"Show","lists":[]}}`))
		default:
			t.Errorf("unexpected provider request: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	items := make([]*movies.Movie, 1001)
	for i := range items {
		id := strconv.Itoa(i + 1)
		items[i] = &movies.Movie{ID: id, TMDBID: &id}
	}
	showID := "1"
	movieLib := movieLibrary{items}
	showLib := showLibrary{[]*series.Series{{ID: "show", TVDBID: &showID}}}
	newService := func() *Service {
		return NewService(store, tmdb.NewClient(tmdb.Config{APIKey: "test", BaseURL: server.URL}), tvdb.NewClient(tvdb.Config{APIKey: "test", BaseURL: server.URL}), movieLib, showLib, nil)
	}
	s := newService()
	// No request from a UI is needed to start or populate the queue.
	if err := s.scanBatch(ctx, 0); err != nil {
		t.Fatal(err)
	}
	status, err := s.ScanStatus(ctx)
	if err != nil || status.Total != 1002 || status.Checked != 16 || showCalls.Load() != 1 {
		t.Fatalf("first batch/fairness: %+v, movies=%d shows=%d err=%v", status, movieCalls.Load(), showCalls.Load(), err)
	}
	// A new service instance must resume the persisted queue, not repeat page 1.
	s = newService()
	for i := 0; i < 62; i++ {
		if err := s.scanBatch(ctx, 0); err != nil {
			t.Fatal(err)
		}
	}
	status, err = s.ScanStatus(ctx)
	if err != nil || status.Total != 1002 || status.Checked != 1002 || status.Pending != 0 || status.Failed != 0 || movieCalls.Load() != 1001 || showCalls.Load() != 1 {
		t.Fatalf("complete scan: %+v, movies=%d shows=%d err=%v", status, movieCalls.Load(), showCalls.Load(), err)
	}
	if err := s.scanBatch(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if movieCalls.Load() != 1001 {
		t.Fatal("completed scan immediately repeated provider calls")
	}
	if _, err := s.RequestScan(ctx); err != nil {
		t.Fatal(err)
	}
	if movieCalls.Load() != 1001 {
		t.Fatal("manual action performed provider work synchronously")
	}
	if err := s.scanBatch(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if movieCalls.Load() != 1016 {
		t.Fatal("optional rescan did not restart fair background progression")
	}
}

func TestBackgroundScanLeaseCancellationAndVisibleRetry(t *testing.T) {
	ctx, store := context.Background(), testStore(t)
	id := "1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "provider unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	s := NewService(store, tmdb.NewClient(tmdb.Config{APIKey: "test", BaseURL: server.URL}), nil, movieLibrary{[]*movies.Movie{{ID: "local", TMDBID: &id}}}, showLibrary{}, nil)
	if _, err := store.db.ExecContext(ctx, `UPDATE collection_scan_lock SET lease_until=9999999999`); err != nil {
		t.Fatal(err)
	}

	if err := s.scanBatch(ctx, 0); err != nil {
		t.Fatal(err)
	}
	status, err := s.ScanStatus(ctx)
	if err != nil || status.Total != 0 {
		t.Fatalf("concurrent lease not honored: %+v %v", status, err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE collection_scan_lock SET lease_until=0`); err != nil {
		t.Fatal(err)
	}
	if err := s.scanBatch(ctx, 0); err == nil {
		t.Fatal("provider failure hidden")
	}
	status, err = s.ScanStatus(ctx)
	if err != nil || status.Failed != 1 || status.Pending != 0 || status.Running || len(status.Errors) != 1 {
		t.Fatalf("retry status: %+v %v", status, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.scanBatch(cancelled, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestBackgroundScanRecoversFailedSource(t *testing.T) {
	ctx, store := context.Background(), testStore(t)
	var recoverSource atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movie/1":
			_, _ = w.Write([]byte(`{"id":1,"title":"Movie","belongs_to_collection":{"id":10,"name":"Set"}}`))
		case "/collection/10":
			if !recoverSource.Load() {
				http.Error(w, "unavailable", http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte(`{"id":10,"name":"Set","parts":[{"id":1,"title":"Movie"}]}`))
		default:
			t.Errorf("unexpected provider request: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	id := "1"
	s := NewService(store, tmdb.NewClient(tmdb.Config{APIKey: "test", BaseURL: server.URL}), nil,
		movieLibrary{[]*movies.Movie{{ID: "local", TMDBID: &id}}}, showLibrary{}, nil)
	if err := s.scanBatch(ctx, 0); err == nil {
		t.Fatal("source failure was hidden")
	}
	ref := Source{Provider: "tmdb", Kind: "collection", ExternalID: "10"}
	before, err := store.FindSource(ctx, ref)
	if err != nil || len(before.Sources) != 1 || before.Sources[0].Error == "" {
		t.Fatalf("failed source was not persisted: %+v %v", before, err)
	}
	recoverSource.Store(true)
	// Advance the persisted retry deadline without a manual UI rescan.
	if _, err := store.db.ExecContext(ctx, `UPDATE collection_scan SET next_at=0`); err != nil {
		t.Fatal(err)
	}
	if err := s.scanBatch(ctx, 0); err != nil {
		t.Fatal(err)
	}
	after, err := store.FindSource(ctx, ref)
	if err != nil || after.ID != before.ID || after.Sources[0].Error != "" || len(after.Members) != 1 {
		t.Fatalf("source did not recover in place: %+v %v", after, err)
	}
	status, err := s.ScanStatus(ctx)
	if err != nil || status.Failed != 0 || status.Checked != 1 || status.Pending != 0 {
		t.Fatalf("recovered scan status: %+v %v", status, err)
	}
}
