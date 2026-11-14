package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/connect"
	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/scanner"
	"github.com/ebenderooock/loom/internal/series"
	dbsqlite "github.com/ebenderooock/loom/internal/storage/db/sqlite"
)

func titleActionFixture(t *testing.T, mediaType string) (*Server, string, string) {
	t.Helper()
	s := newTestServer(t)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := s.db.DB().Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	lib := t.TempDir()
	root := filepath.Join(lib, "Actual custom title folder")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	name := "Movie.2026.mkv"
	if mediaType == "series" {
		name = "Custom.S01E01.mkv"
	}
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte("complete"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unknown.S01E02.mkv"), []byte("downloading"), 0600); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO libraries(id,name,path,media_type) VALUES('library','Library',?,?)`, lib, mediaType)
	if mediaType == "movie" {
		exec(`INSERT INTO movies(id,title,year,library_id,status) VALUES('title','Movie',2026,'library','missing')`)
		exec(`INSERT INTO movie_files(id,movie_id,file_path,size) VALUES('registered','title',?,8)`, path)
		s.moviesSvc = movies.NewService(movies.NewRepository(s.db.DB()))
		s.scannerSvc = scanner.New(s.moviesSvc, nil, s.logger)
	} else {
		exec(`INSERT INTO series(id,title,year,overview,library_id) VALUES('title','Series',2026,'','library')`)
		exec(`INSERT INTO seasons(id,series_id,season_number) VALUES('season','title',1)`)
		exec(`INSERT INTO episodes(id,series_id,season_id,episode_number) VALUES('episode','title','season',1),('unknown','title','season',2)`)
		exec(`INSERT INTO episode_files(id,episode_id,series_id,file_path,file_size) VALUES('registered','episode','title',?,8)`, path)
		s.seriesSvc = series.NewService(series.NewRepository(s.db.DB()), "")
		s.seriesScannerSvc = scanner.NewSeriesScanner(s.seriesSvc, s.logger)
	}
	s.libStore = libraries.NewStore(s.db.DB())
	s.connectSvc = connect.NewService(s.db.DB())
	return s, root, path
}

func TestTitlePlexRefreshUsesEnabledServersWithoutPathsOrTokenExposure(t *testing.T) {
	for _, mediaType := range []string{"movie", "series"} {
		t.Run(mediaType, func(t *testing.T) {
			s, root, path := titleActionFixture(t, mediaType)
			failRefresh, calls := false, 0
			const token = "synthetic-test-token"
			plex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("X-Plex-Token") != token || r.URL.RawQuery != "" {
					t.Errorf("Plex auth/path semantics: %s %+v", r.URL.String(), r.Header)
				}
				if r.URL.Path == "/library/sections" {
					w.Header().Set("Content-Type", "application/xml")
					_, _ = w.Write([]byte(`<MediaContainer><Directory key="1"><Location path="Z:\movies"/></Directory><Directory key="2"><Location path="Z:\tv"/></Directory></MediaContainer>`))
					return
				}
				if r.URL.Path != "/library/sections/1/refresh" && r.URL.Path != "/library/sections/2/refresh" {
					t.Errorf("unexpected Plex operation: %s", r.URL.Path)
				}
				if failRefresh {
					w.WriteHeader(http.StatusUnauthorized)
				}
			}))
			defer plex.Close()
			for _, c := range []*connect.Connection{
				{ID: "plex", Name: "Plex", Provider: connect.ProviderPlex, Enabled: true, NotifyOnImport: false, Settings: connect.ProviderSettings{Host: plex.URL, APIKey: token}},
				{ID: "disabled", Name: "Disabled", Provider: connect.ProviderPlex, Enabled: false, Settings: connect.ProviderSettings{Host: "http://invalid.example"}},
			} {
				if err := s.connectSvc.CreateConnection(context.Background(), c); err != nil {
					t.Fatal(err)
				}
			}
			router := chi.NewRouter()
			router.Get("/{id}/actions", s.titleActions(mediaType))
			router.Post("/{id}/sync-plex", s.syncTitlePlex(mediaType))
			w := do(t, router, "GET", "/title/actions")
			var availability titleActionAvailability
			if err := json.Unmarshal(w.Body.Bytes(), &availability); err != nil {
				t.Fatal(err)
			}
			if availability.FolderPath == nil || *availability.FolderPath != root || availability.PlexUnavailableReason != nil || calls != 0 {
				t.Fatalf("readiness should resolve actual folder without calling Plex: %s", w.Body.String())
			}
			w = do(t, router, "POST", "/title/sync-plex")
			if w.Code != 200 || calls != 3 || strings.Contains(w.Body.String(), token) {
				t.Fatalf("refresh result: %d %s, calls=%d", w.Code, w.Body.String(), calls)
			}
			failRefresh = true
			w = do(t, router, "POST", "/title/sync-plex")
			if w.Code != 502 || strings.Contains(w.Body.String(), token) || !strings.Contains(w.Body.String(), "failed") {
				t.Fatalf("failure must be honest and token-free: %d %s", w.Code, w.Body.String())
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			before := calls
			w = do(t, router, "POST", "/title/sync-plex")
			if w.Code != 409 || calls != before {
				t.Fatalf("missing media must not trigger Plex: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestTitleRescanNeverDiscoversUnknownOrDownloadFiles(t *testing.T) {
	for _, mediaType := range []string{"movie", "series"} {
		t.Run(mediaType, func(t *testing.T) {
			s, root, path := titleActionFixture(t, mediaType)
			router := chi.NewRouter()
			router.Post("/{id}/rescan-library", s.rescanTitleLibrary(mediaType))
			w := do(t, router, "POST", "/title/rescan-library")
			var result scanner.ScanResult
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if w.Code != 200 || result.TotalFiles != 1 || result.Matched != 1 || result.Imported != 0 || result.RootFolderPath != root || len(result.Errors) != 0 {
				t.Fatalf("guarded rescan: %d %+v", w.Code, result)
			}
			table := "movie_files"
			if mediaType == "series" {
				table = "episode_files"
			}
			var count int
			if err := s.db.DB().QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 1 {
				t.Fatalf("unregistered files were ingested: %d %v", count, err)
			}
			if err := s.db.DB().QueryRow(`SELECT COUNT(*) FROM import_history`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("download ingestion was triggered: %d %v", count, err)
			}
			outside := filepath.Join(t.TempDir(), "downloading.mkv")
			if err := os.WriteFile(outside, []byte("complete"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := s.db.DB().QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE file_path=?`, path).Scan(&count); err != nil || count != 1 {
				t.Fatal("registered file identity changed")
			}
			if _, err := s.db.DB().Exec(`UPDATE `+table+` SET file_path=?`, outside); err != nil {
				t.Fatal(err)
			}
			w = do(t, router, "POST", "/title/rescan-library")
			if w.Code != 409 {
				t.Fatalf("download/outside library path should be rejected: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestTitleActionsRequireAdministrator(t *testing.T) {
	s := newTestServer(t)
	store := auth.SQLiteStore{Q: dbsqlite.New(s.db.DB()), DB: s.db.DB()}
	var err error
	s.authSvc, err = auth.NewService(auth.ServiceOptions{
		Store: store, AppConfig: s.appCfg, AppConfigPath: filepath.Join(t.TempDir(), "auth.yaml"),
		SessionSecret: []byte("synthetic-session-secret"), Logger: s.logger,
	})
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser(context.Background(), auth.CreateUserParams{Username: "reader", PasswordHash: "synthetic", Role: "user"})
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := auth.SignSession(s.authSvc.SessionSecret(), auth.SessionPayload{UID: user.ID, IAT: time.Now().Unix(), EXP: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	for _, mediaType := range []string{"movie", "series"} {
		router := chi.NewRouter()
		s.registerTitleActions(router, mediaType)
		for _, endpoint := range []string{"actions", "sync-plex", "rescan-library", "refresh-metadata"} {
			method := "POST"
			if endpoint == "actions" {
				method = "GET"
			}
			w := do(t, router, method, "/title/"+endpoint)
			if w.Code != 401 {
				t.Fatalf("anonymous %s: %d", endpoint, w.Code)
			}
			req := httptest.NewRequest(method, "/title/"+endpoint, nil)
			req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: cookie})
			w = httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != 403 {
				t.Fatalf("nonadmin %s: %d", endpoint, w.Code)
			}
		}
	}
}
