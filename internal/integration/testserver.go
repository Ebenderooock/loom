//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/apikeys"
	"github.com/ebenderooock/loom/internal/appconfig"
	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/importlists"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/kernel/eventbus"
	"github.com/ebenderooock/loom/internal/kernel/telemetry"
	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/metadata"
	"github.com/ebenderooock/loom/internal/metadata/musicbrainz"
	"github.com/ebenderooock/loom/internal/metadata/tmdb"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/music"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
	"github.com/ebenderooock/loom/internal/server"
	"github.com/ebenderooock/loom/internal/storage"
)

type TestServer struct {
	URL string

	db     storage.DB
	server *httptest.Server

	tmdbStub *httptest.Server
	tvdbStub *httptest.Server
	mbStub   *httptest.Server

	client *http.Client
}

func New(t testing.TB) *TestServer {
	t.Helper()

	tmdbStub := newTMDBStub(t)
	tvdbStub := newTVDBStub(t)
	mbStub := newMusicBrainzStub(t)
	rewriteExternalHostsForTests(t, tmdbStub.URL)

	cfgDir := t.TempDir()
	dataDir := t.TempDir()
	_ = os.Setenv("LOOM_CONFIG_DIR", cfgDir)
	_ = os.Setenv("LOOM_DATA_DIR", dataDir)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.HotReload = false
	cfg.RateLimit.Enabled = false
	cfg.Storage.Engine = "sqlite"
	cfg.Storage.SQLite.Path = ":memory:"

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	tel, err := telemetry.New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("init telemetry: %v", err)
	}
	t.Cleanup(func() { _ = tel.Shutdown(context.Background()) })

	db, err := storage.Open(context.Background(), cfg.Storage, logger)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate storage: %v", err)
	}

	appCfg := appconfig.NewDefault()
	appCfgPath := filepath.Join(cfg.ConfigDir, "loom.test.json")
	hash, err := auth.HashPassword("testpassword")
	if err != nil {
		t.Fatalf("hash admin password: %v", err)
	}
	appCfg.SetupComplete = true
	appCfg.Admin.Username = "admin"
	appCfg.Admin.PasswordHash = hash
	if err := appCfg.Save(appCfgPath); err != nil {
		t.Fatalf("save app config: %v", err)
	}

	authStore, err := auth.StoreFromDB(db)
	if err != nil {
		t.Fatalf("auth store: %v", err)
	}
	secret, err := auth.LoadOrCreateSessionSecret(context.Background(), authStore, "integration-session-secret-0123456789", logger)
	if err != nil {
		t.Fatalf("session secret: %v", err)
	}
	authSvc, err := auth.NewService(auth.ServiceOptions{
		Store:         authStore,
		Logger:        logger,
		AppConfig:     appCfg,
		AppConfigPath: appCfgPath,
		SessionSecret: secret,
		SessionTTL:    30 * 24 * time.Hour,
		CookieSecure:  false,
		Invites:       auth.NewInviteStore(db.DB()),
	})
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	if _, err := authSvc.ReconcileAdmin(context.Background()); err != nil {
		t.Fatalf("reconcile admin: %v", err)
	}

	bus := eventbus.NewInProc()
	tmdbClient := tmdb.NewClient(tmdb.Config{
		APIKey:  "integration-key",
		BaseURL: tmdbStub.URL + "/3",
		Timeout: 5 * time.Second,
	})
	metaRepo := metadata.NewSQLiteRepository(db.DB())
	metaSvc := metadata.NewService(metaRepo, []metadata.MetadataProvider{tmdb.NewProvider(tmdbClient)})
	movieRepo := movies.NewRepository(db.DB())
	moviesSvc := movies.NewService(movieRepo, movies.WithMetadata(metaSvc), movies.WithCredits(tmdbClient), movies.WithEventBus(bus))
	movies.SeedDefaults(context.Background(), moviesSvc)

	libStore := libraries.NewStore(db.DB())
	libScanner := libraries.NewScanner(libStore, logger)

	qpStore := qualityprofiles.NewStore(db.DB())
	qualityprofiles.SeedDefaults(context.Background(), qpStore, moviesSvc)

	mbClient := musicbrainz.NewClient(&musicbrainz.Config{
		BaseURL: mbStub.URL + "/ws/2",
		Timeout: 5 * time.Second,
	})
	mbProvider := musicbrainz.NewProvider(mbClient)
	musicSvc := music.NewService(music.NewRepository(db.DB()), mbProvider, logger)

	importStore := importlists.NewStore(db.DB())
	importSync := importlists.NewSyncManager(importStore, logger)
	importSync.SetTMDBAPIKey("integration-key")
	importSync.SetTMDBClient(tmdbClient)
	importSync.SetMoviesService(moviesSvc)
	importSync.SetMusicService(musicSvc)
	importSync.SetLibraryService(libStore)

	srv, err := server.New(cfg, server.Wiring{
		AppConfig:       appCfg,
		Logger:          logger,
		Telemetry:       tel,
		DB:              db,
		Bus:             bus,
		AuthService:     authSvc,
		MoviesService:   moviesSvc,
		LibraryStore:    libStore,
		LibraryScanner:  libScanner,
		QualityProfile:  qpStore,
		ImportListStore: importStore,
		ImportListSync:  importSync,
		APIKeyStore:     apikeys.NewStore(db.DB()),
	})
	if err != nil {
		t.Fatalf("init server: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())

	s := &TestServer{
		URL:      ts.URL,
		db:       db,
		server:   ts,
		tmdbStub: tmdbStub,
		tvdbStub: tvdbStub,
		mbStub:   mbStub,
	}
	t.Cleanup(func() {
		ts.Close()
		tmdbStub.Close()
		tvdbStub.Close()
		mbStub.Close()
	})

	s.client = s.newAuthedClient(t)
	return s
}

func (s *TestServer) Client() *http.Client {
	return s.client
}

func (s *TestServer) MustPost(t testing.TB, path string, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, s.URL+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("build POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		t.Fatalf("POST %s failed: %v", path, err)
	}
	return resp
}

func (s *TestServer) MustGet(t testing.TB, path string) *http.Response {
	t.Helper()
	resp, err := s.client.Get(s.URL + path)
	if err != nil {
		t.Fatalf("GET %s failed: %v", path, err)
	}
	return resp
}

func (s *TestServer) newAuthedClient(t testing.TB) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	c := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	payload := map[string]string{"username": "admin", "password": "testpassword"}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, s.URL+"/api/v1/auth/login", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("build login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("login failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return c
}

func newTMDBStub(t testing.TB) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/3/movie/"):
			_, _ = io.WriteString(w, `{"id":123,"title":"Test Movie","release_date":"2024-01-01","overview":"Stub movie","poster_path":"/poster.jpg","genres":[{"id":18,"name":"Drama"}]}`)
		case r.URL.Path == "/3/search/movie":
			_, _ = io.WriteString(w, `{"results":[{"id":123,"title":"Test Movie","release_date":"2024-01-01"}]}`)
		case strings.HasPrefix(r.URL.Path, "/3/list/"):
			_, _ = io.WriteString(w, `{"items":[{"id":123,"title":"Test Movie","release_date":"2024-01-01"}]}`)
		case r.URL.Path == "/3/movie/popular":
			_, _ = io.WriteString(w, `{"results":[{"id":123,"title":"Test Movie","release_date":"2024-01-01"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
}

func newTVDBStub(t testing.TB) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v4/login":
			_, _ = io.WriteString(w, `{"data":{"token":"dev-token"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
}

func newMusicBrainzStub(t testing.TB) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/ws/2/artist/") {
			id := filepath.Base(r.URL.Path)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   id,
				"name": "Test Artist",
			})
			return
		}
		http.NotFound(w, r)
	}))
}

type hostRewriteTransport struct {
	base      http.RoundTripper
	targetURL *url.URL
}

func (t *hostRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	host := strings.ToLower(req.URL.Hostname())
	if host == "api.themoviedb.org" {
		clone := req.Clone(req.Context())
		u := *clone.URL
		u.Scheme = t.targetURL.Scheme
		u.Host = t.targetURL.Host
		clone.URL = &u
		clone.Host = t.targetURL.Host
		return t.base.RoundTrip(clone)
	}
	return t.base.RoundTrip(req)
}

func rewriteExternalHostsForTests(t testing.TB, targetBaseURL string) {
	t.Helper()
	target, err := url.Parse(targetBaseURL)
	if err != nil {
		t.Fatalf("parse stub base url: %v", err)
	}
	orig := http.DefaultTransport
	base, ok := orig.(http.RoundTripper)
	if !ok || base == nil {
		base = http.DefaultTransport
	}
	http.DefaultTransport = &hostRewriteTransport{
		base:      base,
		targetURL: target,
	}
	t.Cleanup(func() {
		http.DefaultTransport = orig
	})
}
