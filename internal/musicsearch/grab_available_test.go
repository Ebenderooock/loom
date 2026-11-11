package musicsearch

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/storage"
)

type recordingClient struct {
	downloads.DownloadClient
	mu     sync.Mutex
	titles []string
}

func (c *recordingClient) ID() string                   { return "torrent" }
func (c *recordingClient) Name() string                 { return "Torrent" }
func (c *recordingClient) Protocol() downloads.Protocol { return downloads.ProtocolTorrent }
func (c *recordingClient) Add(_ context.Context, req downloads.AddRequest) (downloads.AddResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.titles = append(c.titles, req.Title)
	return downloads.AddResult{ClientID: "torrent", ItemID: req.Title}, nil
}

func historyRegistry(t *testing.T, blockedHashes ...string) (*downloads.Registry, *recordingClient) {
	t.Helper()
	db, err := storage.Open(context.Background(), config.StorageConfig{
		Engine: "sqlite", SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "loom.db")},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	r := downloads.NewRegistry()
	client := &recordingClient{}
	if err := r.Register(client); err != nil {
		t.Fatal(err)
	}
	r.SetHistoryStore(downloads.NewHistoryStore(db.DB()))
	for _, hash := range blockedHashes {
		if _, err := db.DB().Exec(`INSERT INTO blocklist (id, title, indexer_id, release_hash, reason, created_at)
			VALUES (?, 'Blocked release', 'indexer', ?, 'User blocked', CURRENT_TIMESTAMP)`, hash, hash); err != nil {
			t.Fatal(err)
		}
	}
	return r, client
}

func magnetCandidate(title, hash string) Candidate {
	return Candidate{Result: indexers.Result{
		Title: title, IndexerID: "indexer", Infohash: hash,
		MagnetURI: "magnet:?xt=urn:btih:" + hash,
	}}
}

func TestGrabFirstAvailableSkipsAlreadyAcquiredRelease(t *testing.T) {
	registry, client := historyRegistry(t)
	engine := NewEngine(nil, registry, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	first := magnetCandidate("Artist - Album FLAC", "0123456789abcdef0123456789abcdef01234567")
	second := magnetCandidate("Artist - Album MP3", "89abcdef0123456789abcdef0123456789abcdef")

	if _, err := engine.grabFirstAvailable(context.Background(), "album", []Candidate{first, second}); err != nil {
		t.Fatal(err)
	}
	// A failed import leaves the first release acquired; searching again must
	// move on instead of reporting the deferred duplicate.
	grab, err := engine.grabFirstAvailable(context.Background(), "album", []Candidate{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if grab.Title != second.Result.Title {
		t.Fatalf("grabbed %q, want next release", grab.Title)
	}
	if len(client.titles) != 2 {
		t.Fatalf("client calls = %v", client.titles)
	}

	_, err = engine.grabFirstAvailable(context.Background(), "album", []Candidate{first, second})
	if !errors.Is(err, ErrAllReleasesAttempted) {
		t.Fatalf("expected all attempted, got %v", err)
	}
	rec := httptest.NewRecorder()
	writeSearchErr(rec, err)
	if rec.Code != 409 {
		t.Fatalf("status = %d", rec.Code)
	}
	var conflict *downloads.AcquisitionConflict
	if !errors.As(err, &conflict) || !conflict.RetryAllowed || conflict.Title != first.Result.Title {
		t.Fatalf("automatic search lost the first release's recovery decision: %v", err)
	}
	if !strings.Contains(rec.Body.String(), conflict.HistoryID) || !strings.Contains(rec.Body.String(), `"acquisition_deferred"`) {
		t.Fatalf("response does not expose in-place recovery: %s", rec.Body.String())
	}
}
