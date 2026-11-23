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

func TestTitleMediaServerRefreshUsesEnabledServersWithoutPathsOrTokenExposure(t *testing.T) {
	for _, target := range []struct {
		provider connect.ProviderType
		name     string
		calls    int
	}{
		{connect.ProviderPlex, "Plex", 3},
		{connect.ProviderJellyfin, "Jellyfin", 1},
	} {
		for _, mediaType := range []string{"movie", "series"} {
			t.Run(target.name+"/"+mediaType, func(t *testing.T) {
				s, root, path := titleActionFixture(t, mediaType)
				failRefresh, calls := false, 0
				const token = "synthetic-test-token"
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.URL.RawQuery != "" || r.ContentLength > 0 {
						t.Errorf("refresh must not send library paths: %s", r.URL.String())
					}
					if target.provider == connect.ProviderJellyfin {
						if r.Method != http.MethodPost || r.URL.Path != "/prefix/Library/Refresh" ||
							r.Header.Get("Authorization") != `MediaBrowser Token="`+token+`"` {
							t.Errorf("incorrect Jellyfin refresh: %s %s", r.Method, r.URL.String())
						}
						if failRefresh {
							w.WriteHeader(http.StatusUnauthorized)
						} else {
							w.WriteHeader(http.StatusNoContent)
						}
						return
					}
					if r.Header.Get("X-Plex-Token") != token || r.Method != http.MethodGet {
						t.Errorf("incorrect Plex refresh: %s %s", r.Method, r.URL.String())
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
				defer server.Close()
				host := server.URL
				if target.provider == connect.ProviderJellyfin {
					host += "/prefix"
				}
				for _, c := range []*connect.Connection{
					{ID: "target", Name: target.name, Provider: target.provider, Enabled: true, NotifyOnImport: false, Settings: connect.ProviderSettings{Host: host, APIKey: token}},
					{ID: "disabled", Name: "Disabled", Provider: target.provider, Enabled: false, Settings: connect.ProviderSettings{Host: "http://invalid.example"}},
					{ID: "other", Name: "Other", Provider: connect.ProviderEmby, Enabled: true, Settings: connect.ProviderSettings{Host: host, APIKey: token}},
				} {
					if err := s.connectSvc.CreateConnection(context.Background(), c); err != nil {
						t.Fatal(err)
					}
				}
				router := chi.NewRouter()
				router.Get("/{id}/actions", s.titleActions(mediaType))
				endpoint := "/title/sync-" + string(target.provider)
				router.Post("/{id}/sync-"+string(target.provider), s.syncTitleMediaServer(mediaType, target.provider, target.name))
				w := do(t, router, "GET", "/title/actions")
				var availability titleActionAvailability
				if err := json.Unmarshal(w.Body.Bytes(), &availability); err != nil {
					t.Fatal(err)
				}
				configured, reason := availability.PlexConfigured, availability.PlexUnavailableReason
				if target.provider == connect.ProviderJellyfin {
					configured, reason = availability.JellyfinConfigured, availability.JellyfinUnavailableReason
				}
				if availability.FolderPath == nil || *availability.FolderPath != root || !configured || reason != nil || calls != 0 {
					t.Fatalf("readiness should resolve actual folder without calling the server: %s", w.Body.String())
				}
				w = do(t, router, "POST", endpoint)
				if w.Code != 200 || calls != target.calls || strings.Contains(w.Body.String(), token) ||
					!strings.Contains(w.Body.String(), target.name+" library refresh requested") {
					t.Fatalf("refresh result: %d %s, calls=%d", w.Code, w.Body.String(), calls)
				}
				failRefresh = true
				w = do(t, router, "POST", endpoint)
				if w.Code != 502 || strings.Contains(w.Body.String(), token) || !strings.Contains(w.Body.String(), "failed") {
					t.Fatalf("failure must be honest and token-free: %d %s", w.Code, w.Body.String())
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				before := calls
				w = do(t, router, "POST", endpoint)
				if w.Code != 409 || calls != before {
					t.Fatalf("missing media must not trigger a refresh: %d %s", w.Code, w.Body.String())
				}
			})
		}
	}
}

func TestTitleMediaServerVisibilityRequiresEnabledConfiguredConnection(t *testing.T) {
	for _, provider := range []connect.ProviderType{connect.ProviderPlex, connect.ProviderJellyfin} {
		for _, mediaType := range []string{"movie", "series"} {
			for _, configuration := range []string{"none", "disabled", "incomplete", "other", "enabled"} {
				t.Run(string(provider)+"/"+mediaType+"/"+configuration, func(t *testing.T) {
					s, root, _ := titleActionFixture(t, mediaType)
					if err := os.Rename(root, root+" moved"); err != nil {
						t.Fatal(err)
					}
					if configuration != "none" {
						connection := &connect.Connection{
							ID: "server", Name: "Server", Provider: provider,
							Enabled:  configuration != "disabled",
							Settings: connect.ProviderSettings{Host: "http://plex.test", APIKey: "synthetic-token"},
						}
						if configuration == "incomplete" {
							connection.Settings.APIKey = ""
						}
						if configuration == "other" {
							connection.Provider = connect.ProviderEmby
						}
						if err := s.connectSvc.CreateConnection(context.Background(), connection); err != nil {
							t.Fatal(err)
						}
					}
					router := chi.NewRouter()
					router.Get("/{id}/actions", s.titleActions(mediaType))
					w := do(t, router, "GET", "/title/actions")
					var availability titleActionAvailability
					if err := json.Unmarshal(w.Body.Bytes(), &availability); err != nil {
						t.Fatal(err)
					}
					configured, reason := availability.PlexConfigured, availability.PlexUnavailableReason
					if provider == connect.ProviderJellyfin {
						configured, reason = availability.JellyfinConfigured, availability.JellyfinUnavailableReason
					}
					if w.Code != http.StatusOK || configured != (configuration == "enabled") {
						t.Fatalf("sync visibility must depend on connection configuration, not media availability: %d %s", w.Code, w.Body.String())
					}
					if reason == nil {
						t.Fatalf("missing media must still block sync: %s", w.Body.String())
					}
				})
			}
		}
	}
}

func TestTitleRescanPreservesScopeAndSkipsSmallFiles(t *testing.T) {
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

func TestTitleRescanDiscoversFilesAndRepairsAvailability(t *testing.T) {
	for _, mediaType := range []string{"movie", "series"} {
		for _, registration := range []string{"missing", "stale", "deleted"} {
			if mediaType == "series" && registration == "deleted" {
				continue
			}
			t.Run(mediaType+"/"+registration, func(t *testing.T) {
				s, root, path := titleActionFixture(t, mediaType)
				table := "movie_files"
				if mediaType == "series" {
					table = "episode_files"
				}
				query := `DELETE FROM ` + table
				switch registration {
				case "stale":
					query = `UPDATE ` + table + ` SET ` + map[string]string{"movie": "size", "series": "file_size"}[mediaType] + `=1`
				case "deleted":
					query = `UPDATE movie_files SET deleted_at=CURRENT_TIMESTAMP`
				}
				if _, err := s.db.DB().Exec(query); err != nil {
					t.Fatal(err)
				}
				if err := os.Truncate(path, 60*1024*1024); err != nil {
					t.Fatal(err)
				}
				if registration == "missing" {
					folderName := "Movie (2026)"
					if mediaType == "series" {
						folderName = "Series (2026)"
					}
					newRoot := filepath.Join(filepath.Dir(root), folderName)
					if err := os.Rename(root, newRoot); err != nil {
						t.Fatal(err)
					}
					path = filepath.Join(newRoot, filepath.Base(path))
					root = newRoot
				}
				for _, name := range []string{"download.part.mkv", "sample.mkv"} {
					file := filepath.Join(root, name)
					if err := os.WriteFile(file, nil, 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Truncate(file, 60*1024*1024); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Symlink(path, filepath.Join(root, "linked.S01E02.mkv")); err != nil {
					t.Fatal(err)
				}
				router := chi.NewRouter()
				router.Get("/{id}/actions", s.titleActions(mediaType))
				router.Post("/{id}/rescan-library", s.rescanTitleLibrary(mediaType))
				router.Post("/{id}/sync-plex", s.syncTitleMediaServer(mediaType, connect.ProviderPlex, "Plex"))
				w := do(t, router, "GET", "/title/actions")
				var availability titleActionAvailability
				if err := json.Unmarshal(w.Body.Bytes(), &availability); err != nil {
					t.Fatal(err)
				}
				if availability.RescanUnavailableReason != nil || availability.PlexUnavailableReason == nil {
					t.Fatalf("rescan must remain available without complete registrations: %s", w.Body.String())
				}
				if w := do(t, router, "POST", "/title/sync-plex"); w.Code != http.StatusConflict {
					t.Fatalf("Plex must remain blocked: %d %s", w.Code, w.Body.String())
				}
				for range 2 {
					w = do(t, router, "POST", "/title/rescan-library")
					var result scanner.ScanResult
					if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					if w.Code != http.StatusOK || result.Status != scanner.ScanStatusCompleted || result.TotalFiles != 1 || result.Matched != 1 {
						t.Fatalf("folder rescan: %d %+v", w.Code, result)
					}
				}
				var count int
				if err := s.db.DB().QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 1 {
					t.Fatalf("rescan should register only the selected video, without duplicates: %d %v", count, err)
				}
				if mediaType == "movie" {
					file, err := s.moviesSvc.GetMovieFileByPath(context.Background(), path)
					if err != nil || file == nil || file.MovieID != "title" || file.Size != 60*1024*1024 {
						t.Fatalf("movie registration not repaired: %+v %v", file, err)
					}
					movie, err := s.moviesSvc.GetMovie(context.Background(), "title")
					if err != nil || movie.Status != movies.MovieStatusAvailableRightQuality {
						t.Fatalf("movie availability not repaired: %+v %v", movie, err)
					}
				} else {
					var size int64
					if err := s.db.DB().QueryRow(`SELECT file_size FROM episode_files WHERE episode_id='episode' AND file_path=?`, path).Scan(&size); err != nil || size != 60*1024*1024 {
						t.Fatalf("episode registration not repaired: %d %v", size, err)
					}
					episode, err := s.seriesSvc.GetEpisode(context.Background(), "episode")
					if err != nil || !episode.HasFile {
						t.Fatalf("episode availability not repaired: %+v %v", episode, err)
					}
				}
			})
		}
	}
}

func TestTitleRescanRemainsAvailableBeforeFolderExists(t *testing.T) {
	for _, mediaType := range []string{"movie", "series"} {
		t.Run(mediaType, func(t *testing.T) {
			s, root, _ := titleActionFixture(t, mediaType)
			if err := os.Rename(root, root+" moved"); err != nil {
				t.Fatal(err)
			}
			router := chi.NewRouter()
			router.Get("/{id}/actions", s.titleActions(mediaType))
			router.Post("/{id}/rescan-library", s.rescanTitleLibrary(mediaType))
			w := do(t, router, "GET", "/title/actions")
			var availability titleActionAvailability
			if err := json.Unmarshal(w.Body.Bytes(), &availability); err != nil {
				t.Fatal(err)
			}
			if availability.RescanUnavailableReason != nil || availability.FolderPath != nil || availability.PlexUnavailableReason == nil {
				t.Fatalf("only rescan should remain available: %s", w.Body.String())
			}
			w = do(t, router, "POST", "/title/rescan-library")
			if w.Code != http.StatusConflict {
				t.Fatalf("missing folder must be reported honestly: %d %s", w.Code, w.Body.String())
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
	t.Cleanup(s.authSvc.Close)
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
		for _, endpoint := range []string{"actions", "sync-plex", "sync-jellyfin", "rescan-library", "refresh-metadata"} {
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
