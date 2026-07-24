package libraries

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/storage"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func openLibraryTestStore(t *testing.T) *Store {
	t.Helper()

	cfg := config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{
			Path: filepath.Join(t.TempDir(), "libraries-test.db"),
		},
	}
	db, err := storage.Open(context.Background(), cfg, testLogger())
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return NewStore(db.DB())
}

func TestLibraryRouter_UnmappedRouteRemoved(t *testing.T) {
	t.Parallel()

	store := openLibraryTestStore(t)
	router := Router(store, NewScanner(store, testLogger()), testLogger())

	req := httptest.NewRequest(http.MethodGet, "/missing/unmapped", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for removed unmapped route, got %d", rec.Code)
	}
}

func TestListLibraries_OmitsUnmappedCountAndKeepsFileCount(t *testing.T) {
	t.Parallel()

	store := openLibraryTestStore(t)
	libPath := t.TempDir()
	lib := &Library{
		Name:             "Movies",
		Path:             libPath,
		MediaType:        "movie",
		MonitorOnAdd:     true,
		QualityProfileID: "default",
	}
	if err := store.Create(context.Background(), lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := store.UpsertFile(context.Background(), &LibraryFile{
		LibraryID: lib.ID,
		Path:      filepath.Join(libPath, "movie.mkv"),
		SizeBytes: 1234,
	}); err != nil {
		t.Fatalf("upsert file: %v", err)
	}

	router := Router(store, NewScanner(store, testLogger()), testLogger())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Data) != 1 {
		t.Fatalf("expected 1 library, got %d", len(payload.Data))
	}
	if _, exists := payload.Data[0]["unmapped_count"]; exists {
		t.Fatalf("unmapped_count should not be present in response")
	}
	if got, ok := payload.Data[0]["file_count"].(float64); !ok || int(got) != 1 {
		t.Fatalf("expected file_count=1, got %#v", payload.Data[0]["file_count"])
	}
}
