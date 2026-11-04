package music

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/storage"
)

type recordingApplier struct {
	applied []MetadataSource
	tested  []MetadataSource
}

func (a *recordingApplier) Apply(src MetadataSource) { a.applied = append(a.applied, src) }

func (a *recordingApplier) Test(_ context.Context, src MetadataSource) MetadataSourceTestResult {
	a.tested = append(a.tested, src)
	return MetadataSourceTestResult{OK: true, LookupOK: true}
}

func newMetadataSourceTestService(t *testing.T) (*MetadataSourceService, *recordingApplier) {
	t.Helper()
	cfg := config.StorageConfig{
		Engine: "sqlite",
		SQLite: config.SQLiteConfig{Path: filepath.Join(t.TempDir(), "music_source.db")},
	}
	db, err := storage.Open(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	applier := &recordingApplier{}
	return NewMetadataSourceService(NewMetadataSourceStore(db.DB()), applier), applier
}

func TestMetadataSourceDefaultsToPublicMusicBrainz(t *testing.T) {
	svc, applier := newMetadataSourceTestService(t)
	src, err := svc.ApplyStored(context.Background())
	if err != nil {
		t.Fatalf("ApplyStored: %v", err)
	}
	if src.Mode != MetadataSourceMusicBrainz || !src.SearchOnPublic || src.AuthHeaderSet {
		t.Fatalf("unexpected default source: %+v", src)
	}
	if len(applier.applied) != 1 {
		t.Fatalf("expected stored source to be applied once, got %d", len(applier.applied))
	}
}

func TestMetadataSourceUpdatePersistsAndKeepsSecret(t *testing.T) {
	ctx := context.Background()
	svc, applier := newMetadataSourceTestService(t)

	saved, err := svc.Update(ctx, MetadataSourceUpdate{
		Mode: MetadataSourceCustom, BaseURL: "https://mb.example.com/ws/2/", RateLimitMS: 250,
		AuthHeaderName: "x-api-key", AuthHeaderValue: "secret", SearchOnPublic: false,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if saved.BaseURL != "https://mb.example.com/ws/2" || saved.AuthHeaderName != "X-Api-Key" || !saved.AuthHeaderSet {
		t.Fatalf("unexpected saved source: %+v", saved)
	}
	if got := applier.applied[len(applier.applied)-1]; got.AuthHeaderValue != "secret" || got.RateLimitMS != 250 {
		t.Fatalf("applier did not receive saved source: %+v", got)
	}

	// An empty value keeps the stored secret.
	if _, err := svc.Update(ctx, MetadataSourceUpdate{
		Mode: MetadataSourceCustom, BaseURL: "http://mirror.lan:5000/ws/2", AuthHeaderName: "X-Api-Key",
	}); err != nil {
		t.Fatalf("Update keep secret: %v", err)
	}
	got, err := svc.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AuthHeaderValue != "secret" || got.BaseURL != "http://mirror.lan:5000/ws/2" {
		t.Fatalf("secret or URL not persisted: %+v", got)
	}

	// Switching back to public keeps the custom settings for later.
	if _, err := svc.Update(ctx, MetadataSourceUpdate{Mode: MetadataSourceMusicBrainz}); err != nil {
		t.Fatalf("Update public: %v", err)
	}
	got, _ = svc.Get(ctx)
	if got.Mode != MetadataSourceMusicBrainz || got.BaseURL != "http://mirror.lan:5000/ws/2" {
		t.Fatalf("expected public mode with retained custom URL: %+v", got)
	}

	if _, err := svc.Update(ctx, MetadataSourceUpdate{
		Mode: MetadataSourceCustom, BaseURL: "http://mirror.lan:5000/ws/2", AuthHeaderName: "X-Api-Key", ClearAuthHeader: true,
	}); err != nil {
		t.Fatalf("Update clear: %v", err)
	}
	got, _ = svc.Get(ctx)
	if got.AuthHeaderValue != "" || got.AuthHeaderSet {
		t.Fatalf("expected secret to be cleared: %+v", got)
	}
}

func TestMetadataSourceUpdateValidates(t *testing.T) {
	svc, _ := newMetadataSourceTestService(t)
	cases := []MetadataSourceUpdate{
		{Mode: "lidarr"},
		{Mode: MetadataSourceCustom, BaseURL: "ftp://mirror/ws/2"},
		{Mode: MetadataSourceCustom, BaseURL: "not a url"},
		{Mode: MetadataSourceCustom, BaseURL: "http://mirror/ws/2", RateLimitMS: -1},
		{Mode: MetadataSourceCustom, BaseURL: "http://mirror/ws/2", AuthHeaderName: "Bad Header"},
		{Mode: MetadataSourceCustom, BaseURL: "http://mirror/ws/2", AuthHeaderName: "X-Key", AuthHeaderValue: "a\r\nb"},
	}
	for _, upd := range cases {
		if _, err := svc.Update(context.Background(), upd); !errors.Is(err, ErrInvalid) {
			t.Errorf("Update(%+v) error = %v, want ErrInvalid", upd, err)
		}
	}
}

func TestMetadataSourceRouterHidesSecret(t *testing.T) {
	svc, applier := newMetadataSourceTestService(t)
	router := MetadataSourceRouter(svc)

	body := `{"mode":"custom","base_url":"https://mb.example.com/ws/2","auth_header_name":"Authorization","auth_header_value":"Bearer token","search_on_public":true}`
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Bearer token") {
		t.Fatalf("PUT response leaked secret: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if got["auth_header_set"] != true || strings.Contains(rec.Body.String(), "Bearer token") {
		t.Fatalf("unexpected GET response: %s", rec.Body.String())
	}

	// Test reuses the stored secret when none is supplied.
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/test",
		strings.NewReader(`{"mode":"custom","base_url":"https://mb.example.com/ws/2","auth_header_name":"Authorization"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /test status = %d: %s", rec.Code, rec.Body.String())
	}
	if len(applier.tested) != 1 || applier.tested[0].AuthHeaderValue != "Bearer token" {
		t.Fatalf("test did not reuse stored secret: %+v", applier.tested)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"mode":"custom","base_url":"ftp://x"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid PUT status = %d, want 400", rec.Code)
	}
}
