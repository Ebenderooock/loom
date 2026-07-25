package imports

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/kernel/eventbus"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/series"
)

type moviesSvcStub struct{ movies.Service }
type seriesSvcStub struct{ series.Service }

func newDownloadsSvcForPipelineTest(t *testing.T) *downloads.Service {
	t.Helper()
	svc, err := downloads.NewService(downloads.ServiceOptions{
		Repository: resolvePathFakeRepo{},
		Registry:   downloads.NewRegistry(),
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("downloads.NewService: %v", err)
	}
	return svc
}

func basePipelineOptions(t *testing.T) PipelineOptions {
	t.Helper()
	db := setupTestDB(t)
	return PipelineOptions{
		DB:          db,
		Bus:         eventbus.NewInProc(),
		DownloadSvc: newDownloadsSvcForPipelineTest(t),
		MoviesSvc:   &moviesSvcStub{},
		SeriesSvc:   &seriesSvcStub{},
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestNewPipeline_ValidatesRequiredDependencies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mut  func(*PipelineOptions)
		want string
	}{
		{"missing db", func(o *PipelineOptions) { o.DB = nil }, "db required"},
		{"missing bus", func(o *PipelineOptions) { o.Bus = nil }, "event bus required"},
		{"missing downloads", func(o *PipelineOptions) { o.DownloadSvc = nil }, "download service required"},
		{"missing movies", func(o *PipelineOptions) { o.MoviesSvc = nil }, "movies service required"},
		{"missing series", func(o *PipelineOptions) { o.SeriesSvc = nil }, "series service required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := basePipelineOptions(t)
			tt.mut(&opts)
			if _, err := NewPipeline(opts); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestNewPipeline_DefaultImportMode(t *testing.T) {
	t.Parallel()
	opts := basePipelineOptions(t)
	opts.ImportMode = ""
	p, err := NewPipeline(opts)
	if err != nil {
		t.Fatalf("NewPipeline: %v", err)
	}
	if p.importMode != ImportModeMove {
		t.Fatalf("default import mode = %q, want %q", p.importMode, ImportModeMove)
	}
}

func TestPipelineUtilityHelpers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want bool
	}{
		{"/downloads/infohash:abc123", true},
		{" /downloads/infohash:abc123 ", true},
		{"/downloads/normal/path", false},
	}
	for _, tt := range tests {
		if got := isUnresolvedContentPath(tt.path); got != tt.want {
			t.Fatalf("isUnresolvedContentPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}

	if got := metadataString(`{"content_path":"/a/b","n":1}`, "content_path"); got != "/a/b" {
		t.Fatalf("metadataString(content_path) = %q", got)
	}
	if got := metadataString(`{"content_path":123}`, "content_path"); got != "" {
		t.Fatalf("metadataString should return empty for non-string value, got %q", got)
	}
	if got := metadataString("not-json", "content_path"); got != "" {
		t.Fatalf("metadataString should return empty for invalid json, got %q", got)
	}
}

func TestProcessImport_PathMissingRecordsFailure(t *testing.T) {
	t.Parallel()
	opts := basePipelineOptions(t)
	p, err := NewPipeline(opts)
	if err != nil {
		t.Fatalf("NewPipeline: %v", err)
	}

	missingPath := "/tmp/path-that-does-not-exist"
	err = p.processImport(context.Background(), &downloads.DownloadCompletedEvent{Title: "Missing"}, missingPath)
	if err == nil || !strings.Contains(err.Error(), "download path not found") {
		t.Fatalf("expected missing path error, got %v", err)
	}

	var status string
	rowErr := opts.DB.QueryRow(`SELECT status FROM import_history LIMIT 1`).Scan(&status)
	if rowErr != nil {
		t.Fatalf("expected failure row in import_history: %v", rowErr)
	}
	if status != string(StatusFailed) {
		t.Fatalf("recorded status = %q, want %q", status, StatusFailed)
	}
}
