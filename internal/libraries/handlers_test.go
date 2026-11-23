package libraries

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/storage"
)

func TestLibraryHandlers_FileStatsFailure(t *testing.T) {
	store := openLibraryTestStore(t)
	lib := &Library{Name: "Movies", Path: t.TempDir(), MediaType: "movie", QualityProfileID: "default"}
	if err := store.Create(context.Background(), lib); err != nil {
		t.Fatal(err)
	}
	// Library reads and writes still work, but the stats query cannot succeed.
	if _, err := store.db.ExecContext(context.Background(), `DROP TABLE library_files`); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	router := Router(store, NewScanner(store, logger), logger)
	createBody, err := json.Marshal(CreateLibraryRequest{Name: "Other", Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, method, path, body string
	}{
		{"list", http.MethodGet, "/", ""},
		{"detail", http.MethodGet, "/" + lib.ID, ""},
		{"create", http.MethodPost, "/", string(createBody)},
		{"update", http.MethodPut, "/" + lib.ID, "{}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs.Reset()
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
			}
			var payload struct {
				Error struct {
					Code, Message string
				} `json:"error"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.Error.Code != "Internal Server Error" || payload.Error.Message != "internal error" {
				t.Fatalf("unexpected error response: %+v", payload)
			}
			if !strings.Contains(logs.String(), "libraries: file stats") || !strings.Contains(logs.String(), "library_files") {
				t.Fatalf("missing stats error log: %s", logs.String())
			}
		})
	}
}

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
	if got := payload.Data[0]["size_bytes"]; got != float64(1234) {
		t.Fatalf("expected size_bytes=1234, got %#v", got)
	}
	disk, ok := payload.Data[0]["disk_space"].(map[string]any)
	if !ok || disk["volume_id"] == "" || disk["volume_id"] == nil {
		t.Fatalf("expected volume_id, got %#v", payload.Data[0]["disk_space"])
	}
}

func TestLibrarySizeTracksScannedFiles(t *testing.T) {
	store := openLibraryTestStore(t)
	root := t.TempDir()
	lib := &Library{Name: "Movies", Path: root, MediaType: "movie", QualityProfileID: "default"}
	if err := store.Create(context.Background(), lib); err != nil {
		t.Fatal(err)
	}
	other := &Library{Name: "Other", Path: t.TempDir(), MediaType: "movie", QualityProfileID: "default"}
	if err := store.Create(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFile(context.Background(), &LibraryFile{LibraryID: other.ID, Path: filepath.Join(other.Path, "other.mkv"), SizeBytes: 100}); err != nil {
		t.Fatal(err)
	}
	router := Router(store, NewScanner(store, testLogger()), testLogger())
	check := func(count, size int64) {
		t.Helper()
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+lib.ID, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("detail status: %d", rec.Code)
		}
		var payload struct {
			Library Library `json:"library"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if int64(payload.Library.FileCount) != count || payload.Library.SizeBytes != size {
			t.Fatalf("got count %d size %d; want %d %d", payload.Library.FileCount, payload.Library.SizeBytes, count, size)
		}
	}
	check(0, 0)
	file := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(store, testLogger())
	if err := scanner.ScanLibrary(context.Background(), lib); err != nil {
		t.Fatal(err)
	}
	check(1, 5)
	if err := os.WriteFile(file, []byte("hello world"), 0o600); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(root, "second.mp4")
	if err := os.WriteFile(second, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := scanner.ScanLibrary(context.Background(), lib); err != nil {
		t.Fatal(err)
	}
	check(2, 14)
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := scanner.ScanLibrary(context.Background(), lib); err != nil {
		t.Fatal(err)
	}
	check(1, 3)
	if err := os.Remove(second); err != nil {
		t.Fatal(err)
	}
	if err := scanner.ScanLibrary(context.Background(), lib); err != nil {
		t.Fatal(err)
	}
	check(0, 0)
}
