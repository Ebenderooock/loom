package connect_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/connect"
	"github.com/ebenderooock/loom/internal/imports"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/kernel/eventbus"
	"github.com/ebenderooock/loom/internal/storage"
)

func testService(t *testing.T) connect.Service {
	t.Helper()
	db, err := storage.Open(context.Background(), config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "loom.db")},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return connect.NewService(db.DB())
}

type topicEvent string

func (e topicEvent) Topic() string { return string(e) }

func TestCompletedImportRefreshesConfiguredProviders(t *testing.T) {
	var mu sync.Mutex
	requests := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests[r.URL.Path]++
		if r.URL.RawQuery != "" {
			t.Error("must not send Loom's Linux import path to Windows Plex")
		}
		switch r.URL.Path {
		case "/plex/library/sections":
			if r.Method != http.MethodGet || r.Header.Get("X-Plex-Token") != "synthetic-token" {
				t.Error("incorrect Plex listing request")
			}
			_, _ = fmt.Fprint(w, `<MediaContainer><Directory key="4" type="movie"><Location path="Z:\movies"/></Directory><Directory key="3" type="show"><Location path="Z:\tv"/></Directory></MediaContainer>`)
		case "/plex/library/sections/4/refresh", "/plex/library/sections/3/refresh":
			if r.Method != http.MethodGet || r.Header.Get("X-Plex-Token") != "synthetic-token" {
				t.Error("incorrect Plex refresh request")
			}
		case "/jellyfin/Library/Refresh":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != `MediaBrowser Token="synthetic-token"` {
				t.Error("incorrect Jellyfin refresh request")
			}
		case "/emby/Library/Refresh":
			if r.Method != http.MethodPost || r.Header.Get("X-Emby-Token") != "synthetic-token" {
				t.Error("incorrect Emby refresh request")
			}
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := testService(t)
	for _, c := range []*connect.Connection{
		{Name: "Plex", Provider: connect.ProviderPlex, Enabled: true, NotifyOnImport: true, Settings: connect.ProviderSettings{Host: server.URL + "/plex", APIKey: "synthetic-token"}},
		{Name: "Jellyfin", Provider: connect.ProviderJellyfin, Enabled: true, NotifyOnImport: true, Settings: connect.ProviderSettings{Host: server.URL + "/jellyfin", APIKey: "synthetic-token"}},
		{Name: "Emby", Provider: connect.ProviderEmby, Enabled: true, NotifyOnImport: true, Settings: connect.ProviderSettings{Host: server.URL + "/emby", APIKey: "synthetic-token"}},
		{Name: "Trakt", Provider: connect.ProviderTrakt, Enabled: true, NotifyOnImport: true},
		{Name: "disabled", Provider: connect.ProviderPlex, Enabled: false, NotifyOnImport: true},
		{Name: "opted out", Provider: connect.ProviderPlex, Enabled: true, NotifyOnImport: false},
	} {
		if err := svc.CreateConnection(context.Background(), c); err != nil {
			t.Fatal(err)
		}
	}
	bus := eventbus.NewInProc()
	t.Cleanup(func() { _ = bus.Close(time.Second) })
	unsubscribe := connect.SubscribeImportEvents(context.Background(), bus, svc)
	// An async handler must not inherit cancellation of a completed import request.
	publishCtx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ev := range []eventbus.Event{
		topicEvent("downloads.queued"), topicEvent("downloads.completed"),
		&imports.ImportFailedEvent{Title: "synthetic failure"},
		&imports.ImportCompletedEvent{MediaType: imports.MediaTypeMovie, Title: "synthetic import", DestPath: "/media/movies/synthetic.mkv"},
		&imports.ImportCompletedEvent{MediaType: imports.MediaTypeEpisode, Title: "synthetic episode", DestPath: "/media/tv/synthetic.mkv"},
		&imports.ImportCompletedEvent{MediaType: imports.MediaTypeMusic, Title: "synthetic album", DestPath: "/media/music/synthetic.flac"},
	} {
		if err := bus.Publish(publishCtx, ev); err != nil {
			t.Fatal(err)
		}
	}
	unsubscribe()
	if err := bus.Publish(context.Background(), &imports.ImportCompletedEvent{}); err != nil {
		t.Fatal(err)
	}
	if err := bus.Close(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, path := range []string{
		"/plex/library/sections", "/plex/library/sections/4/refresh", "/plex/library/sections/3/refresh",
		"/jellyfin/Library/Refresh", "/emby/Library/Refresh",
	} {
		if requests[path] != 3 {
			t.Errorf("%s: got %d requests, want exactly 3 (one per completed import)", path, requests[path])
		}
	}
	if len(requests) != 5 {
		t.Errorf("unexpected request paths: %v", requests)
	}
}

func TestNotifyAllReturnsFailuresWithoutBlockingOtherProviders(t *testing.T) {
	refreshed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Library/Refresh" {
			refreshed = true
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, "synthetic-token")
	}))
	defer server.Close()
	svc := testService(t)
	for _, c := range []*connect.Connection{
		{Name: "Plex", Provider: connect.ProviderPlex, Enabled: true, NotifyOnImport: true, Settings: connect.ProviderSettings{Host: server.URL, APIKey: "synthetic-token"}},
		{Name: "Unknown", Provider: "invalid", Enabled: true, NotifyOnImport: true},
		{Name: "Jellyfin", Provider: connect.ProviderJellyfin, Enabled: true, NotifyOnImport: true, Settings: connect.ProviderSettings{Host: server.URL, APIKey: "synthetic-token"}},
	} {
		if err := svc.CreateConnection(context.Background(), c); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.NotifyAll(context.Background(), "download"); err != nil || refreshed {
		t.Fatalf("unexpected dispatch for download: %v, refreshed=%v", err, refreshed)
	}
	err := svc.NotifyAll(context.Background(), "import")
	if err == nil || !strings.Contains(err.Error(), "connection Plex: plex sections: unexpected status 401") ||
		!strings.Contains(err.Error(), "connection Unknown: unknown provider type: invalid") {
		t.Fatalf("expected both connection errors, got %v", err)
	}
	if strings.Contains(err.Error(), "synthetic-token") {
		t.Fatal("returned failure exposes token")
	}
	if !refreshed {
		t.Fatal("failing Plex connection prevented Jellyfin refresh")
	}
}
