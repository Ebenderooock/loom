package rss

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/storage"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func openTestDB(t *testing.T) storage.DB {
	t.Helper()
	dir := t.TempDir()
	cfg := config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: filepath.Join(dir, "loom.db")},
	}
	db, err := storage.Open(context.Background(), cfg, quietLogger())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

// TestHandlersCreateSource verifies POST /api/v1/rss/sources works.
func TestHandlersCreateSource(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	logger := quietLogger()

	svc := NewSourcesService(logger, db)
	router := chi.NewRouter()
	svc.Mount(router)

	body := createSourceRequest{
		Name:   "Test RSS Source",
		Type:   SourceTypeRSS,
		Config: json.RawMessage(`{"url":"http://example.com/rss","auth_type":"none"}`),
	}

	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/rss/sources", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var resp UserSource
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Name != "Test RSS Source" {
		t.Errorf("expected name 'Test RSS Source', got '%s'", resp.Name)
	}
	if resp.Type != SourceTypeRSS {
		t.Errorf("expected type %v, got %v", SourceTypeRSS, resp.Type)
	}
}

// TestHandlersListSources verifies GET /api/v1/rss/sources works.
func TestHandlersListSources(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	logger := quietLogger()

	svc := NewSourcesService(logger, db)

	// Create a source first
	_, err := svc.CreateSource(
		context.Background(),
		"rss-test-1",
		"Test Source 1",
		SourceTypeRSS,
		[]byte(`{"url":"http://example.com/rss"}`),
	)
	if err != nil {
		t.Fatalf("failed to create test source: %v", err)
	}

	router := chi.NewRouter()
	svc.Mount(router)

	req := httptest.NewRequest("GET", "/api/v1/rss/sources", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp struct {
		Sources []*UserSource `json:"sources"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Sources) != 1 {
		t.Errorf("expected 1 source, got %d", len(resp.Sources))
	}
}

// TestHandlersGetSource verifies GET /api/v1/rss/sources/{id} works.
func TestHandlersGetSource(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	logger := quietLogger()

	svc := NewSourcesService(logger, db)

	// Create a source first
	created, err := svc.CreateSource(
		context.Background(),
		"rss-test-get",
		"Test Get Source",
		SourceTypeRSS,
		[]byte(`{"url":"http://example.com/rss"}`),
	)
	if err != nil {
		t.Fatalf("failed to create test source: %v", err)
	}

	router := chi.NewRouter()
	svc.Mount(router)

	req := httptest.NewRequest("GET", "/api/v1/rss/sources/"+created.ID, nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp UserSource
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.ID != created.ID {
		t.Errorf("expected ID '%s', got '%s'", created.ID, resp.ID)
	}
	if resp.Name != "Test Get Source" {
		t.Errorf("expected name 'Test Get Source', got '%s'", resp.Name)
	}
}

// TestHandlersDeleteSource verifies DELETE /api/v1/rss/sources/{id} works.
func TestHandlersDeleteSource(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	logger := quietLogger()

	svc := NewSourcesService(logger, db)

	// Create a source first
	created, err := svc.CreateSource(
		context.Background(),
		"rss-test-delete",
		"Test Delete Source",
		SourceTypeRSS,
		[]byte(`{"url":"http://example.com/rss"}`),
	)
	if err != nil {
		t.Fatalf("failed to create test source: %v", err)
	}

	router := chi.NewRouter()
	svc.Mount(router)

	req := httptest.NewRequest("DELETE", "/api/v1/rss/sources/"+created.ID, nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d", http.StatusNoContent, w.Code)
	}

	// Verify it's deleted
	_, err = svc.GetSource(context.Background(), created.ID)
	if err == nil {
		t.Errorf("expected error after delete, got nil")
	}
}

// TestHandlersCreateSourceInvalidConfig verifies validation works.
func TestHandlersCreateSourceInvalidConfig(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	logger := quietLogger()

	svc := NewSourcesService(logger, db)
	router := chi.NewRouter()
	svc.Mount(router)

	body := createSourceRequest{
		Name:   "Invalid Source",
		Type:   SourceTypeRSS,
		Config: json.RawMessage(`{}`), // Empty config is invalid
	}

	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/rss/sources", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// TestHandlersCreateSourceDuplicateName verifies name uniqueness.
func TestHandlersCreateSourceDuplicateName(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	logger := quietLogger()

	svc := NewSourcesService(logger, db)
	router := chi.NewRouter()
	svc.Mount(router)

	body := createSourceRequest{
		Name:   "Duplicate Name",
		Type:   SourceTypeRSS,
		Config: json.RawMessage(`{"url":"http://example.com/rss","auth_type":"none"}`),
	}

	// Create first source
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/rss/sources", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("first create failed: expected %d, got %d", http.StatusCreated, w.Code)
	}

	// Try to create duplicate
	bodyBytes, _ = json.Marshal(body)
	req = httptest.NewRequest("POST", "/api/v1/rss/sources", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected conflict on duplicate, got %d", w.Code)
	}
}
