package titlefiles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/appconfig"
	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/requests"
	"github.com/ebenderooock/loom/internal/storage"
)

type fixture struct {
	s             *Service
	db            *sql.DB
	storage       storage.DB
	library, root string
}

func newFixture(t *testing.T, mediaType string) fixture {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, config.StorageConfig{Engine: "sqlite", SQLite: config.SQLiteConfig{
		Path: filepath.Join(t.TempDir(), "files.db"),
	}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	library := t.TempDir()
	root := filepath.Join(library, "Custom Folder")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	s := New(db.DB(), libraries.NewStore(db.DB()), &appconfig.Config{}, nil, nil)
	f := fixture{s: s, db: db.DB(), storage: db, library: library, root: root}
	f.exec(t, `INSERT INTO libraries(id,name,path,media_type) VALUES('lib','Library',?,?)`, library, mediaType)
	if mediaType == "movie" {
		f.exec(t, `INSERT INTO movies(id,title,library_id,status) VALUES('title','Movie','lib','available_right_quality')`)
		f.movieFile(t, "video", "movie.mkv")
	} else {
		f.exec(t, `INSERT INTO series(id,title,library_id) VALUES('title','Show','lib')`)
		for _, season := range []int{0, 1, 2} {
			f.exec(t, `INSERT INTO seasons(id,series_id,season_number) VALUES(?, 'title',?)`, season, season)
			f.exec(t, `INSERT INTO episodes(id,series_id,season_id,episode_number,has_file) VALUES(?,'title',?,1,true)`, season, season)
		}
		f.episodeFile(t, "video", "Season 01/show.S01E01.mkv", 1)
		f.episodeFile(t, "video2", "Season 02/show.S02E01.mkv", 2)
	}
	f.write(t, "extras/sample.mp4")
	f.write(t, "movie.nfo")
	f.write(t, "subs/movie.en.srt")
	return f
}

func (f fixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func (f fixture) write(t *testing.T, path string) string {
	t.Helper()
	full := filepath.Join(f.root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("media"), 0600); err != nil {
		t.Fatal(err)
	}
	return full
}

func (f fixture) movieFile(t *testing.T, id, path string) {
	t.Helper()
	full := f.write(t, path)
	f.exec(t, `INSERT INTO movie_files(id,movie_id,file_path,quality,format) VALUES(?,'title',?,'1080p','h264')`, id, full)
	f.exec(t, `INSERT INTO library_files(id,library_id,path,media_id) VALUES(?,'lib',?,'title')`, id, full)
}

func (f fixture) episodeFile(t *testing.T, id, path string, season int) {
	t.Helper()
	full := f.write(t, path)
	f.exec(t, `INSERT INTO episode_files(id,episode_id,series_id,file_path,quality,codec) VALUES(?,?,'title',?,'1080p','h264')`, id, season, full)
	f.exec(t, `INSERT INTO library_files(id,library_id,path,media_id) VALUES(?,'lib',?,'title')`, id, full)
}

func (f fixture) count(t *testing.T, query string) int {
	t.Helper()
	var count int
	if err := f.db.QueryRowContext(context.Background(), query).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestListAllDiskFilesAndSeasonScope(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "movie")
	got, err := f.s.List(context.Background(), "movie", "title", nil)
	if err != nil || len(got.Files) != 4 || got.DeletionMode != "permanent" {
		t.Fatalf("listing: %+v, %v", got, err)
	}
	for _, file := range got.Files {
		if filepath.IsAbs(file.Path) || file.Size != 5 {
			t.Fatalf("invalid wire file: %+v", file)
		}
		if file.Path == "movie.mkv" && (!file.Tracked || file.Quality != "1080p" || file.Format != "h264") {
			t.Fatalf("lost tracked metadata: %+v", file)
		}
	}
	show := newFixture(t, "series")
	show.write(t, "Season 01/subs/show.srt")
	show.episodeFile(t, "custom-folder", "Volume One/show.S01E01.mp4", 1)
	show.write(t, "Volume One/subs/episode.srt")
	show.write(t, "Specials/extra.nfo")
	season := 1
	list, err := show.s.List(context.Background(), "series", "title", &season)
	if err != nil || len(list.Files) != 4 {
		t.Fatalf("season listing: %+v %v", list, err)
	}
	for _, file := range list.Files {
		if !strings.HasPrefix(file.Path, "Season 01/") && !strings.HasPrefix(file.Path, "Volume One/") {
			t.Fatalf("wrong season file: %+v", file)
		}
	}
	if _, err := show.s.Delete(context.Background(), "series", "title", &season, []string{"Season 02/show.S02E01.mkv"}, "permanent"); !errors.Is(err, errInvalid) {
		t.Fatalf("cross-season delete: %v", err)
	}
	season = 0
	list, err = show.s.List(context.Background(), "series", "title", &season)
	if err != nil || len(list.Files) != 1 || list.Files[0].Path != "Specials/extra.nfo" {
		t.Fatalf("specials listing: %+v %v", list, err)
	}
}

func TestRejectUnsafeSelectionsBeforeAnyDeletion(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "movie")
	outside := filepath.Join(t.TempDir(), "precious.txt")
	if err := os.WriteFile(outside, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.root, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(f.root, "escape-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(f.root, "movie.mkv"), filepath.Join(f.root, "internal-link")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../precious.txt", outside, "escape", "escape-dir/precious.txt", "internal-link", ".", "extras", `..\precious.txt`, "movie.mkv/../movie.nfo"} {
		deleted, err := f.s.Delete(context.Background(), "movie", "title", nil, []string{"movie.mkv", path}, "permanent")
		if !errors.Is(err, errInvalid) || len(deleted) != 0 {
			t.Errorf("unsafe selection %q: deleted=%v err=%v", path, deleted, err)
		}
	}
	for _, path := range []string{outside, filepath.Join(f.root, "movie.mkv")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("protected file removed: %v", err)
		}
	}
	if _, err := f.s.Delete(context.Background(), "movie", "title", nil, []string{"movie.mkv", "movie.mkv"}, "permanent"); !errors.Is(err, errInvalid) {
		t.Fatalf("duplicate selection accepted: %v", err)
	}
	got, err := f.s.List(context.Background(), "movie", "title", nil)
	if err != nil || len(got.Files) != 4 {
		t.Fatalf("symlinks must not be listed: %+v %v", got, err)
	}
}

func TestRecycleAndPermanentDeleteReconcileMovies(t *testing.T) {
	t.Parallel()
	for _, recycle := range []bool{false, true} {
		t.Run(map[bool]string{false: "permanent", true: "recycle"}[recycle], func(t *testing.T) {
			f := newFixture(t, "movie")
			f.movieFile(t, "alternate", "movie.720p.mkv")
			f.s.cfg.RecycleBin.Enabled = recycle
			f.s.cfg.RecycleBin.Path = t.TempDir()
			mode := f.s.deletionMode()
			ctx := auth.WithIdentity(context.Background(), &auth.Identity{Username: "admin"})
			deleted, err := f.s.Delete(ctx, "movie", "title", nil, []string{"movie.mkv", "movie.nfo"}, mode)
			if err != nil || len(deleted) != 2 {
				t.Fatalf("delete: %v %v", deleted, err)
			}
			if f.count(t, `SELECT COUNT(*) FROM movie_files WHERE id='video' AND deleted_at IS NOT NULL`) != 1 ||
				f.count(t, `SELECT COUNT(*) FROM movies WHERE status='available_right_quality'`) != 1 ||
				f.count(t, `SELECT COUNT(*) FROM library_files WHERE id='video'`) != 0 {
				t.Fatal("alternate movie status or removed file bookkeeping incorrect")
			}
			if f.count(t, `SELECT COUNT(*) FROM audit_log WHERE event_type='file_deleted' AND detail LIKE '%admin%'`) != 2 {
				t.Fatal("missing per-file audit records")
			}
			for _, name := range deleted {
				if _, err := os.Stat(filepath.Join(f.root, name)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("file still on disk: %s %v", name, err)
				}
				target := filepath.Join(f.s.cfg.RecycleBin.Path, filepath.Base(f.library), filepath.Base(f.root), name)
				_, err := os.Stat(target)
				if (recycle && err != nil) || (!recycle && !errors.Is(err, os.ErrNotExist)) {
					t.Fatalf("recycle destination: %s %v", target, err)
				}
			}
			if _, err := f.s.Delete(ctx, "movie", "title", nil, []string{"movie.720p.mkv"}, mode); err != nil {
				t.Fatal(err)
			}
			if f.count(t, `SELECT COUNT(*) FROM movies WHERE status='missing'`) != 1 {
				t.Fatal("last movie file deletion did not mark movie missing")
			}
		})
	}
}

func TestEpisodeStatusAndRemainingExtras(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "series")
	f.episodeFile(t, "alternate", "Season 01/alternate.mkv", 1)
	ctx := context.Background()
	for _, path := range []string{"Season 01/show.S01E01.mkv", "Season 01/alternate.mkv", "Season 02/show.S02E01.mkv"} {
		if _, err := f.s.Delete(ctx, "series", "title", nil, []string{path}, "permanent"); err != nil {
			t.Fatal(err)
		}
		if path == "Season 01/show.S01E01.mkv" && f.count(t, `SELECT COUNT(*) FROM episodes WHERE id='1' AND has_file=true`) != 1 {
			t.Fatal("alternate episode file was ignored")
		}
	}
	if f.count(t, `SELECT COUNT(*) FROM episode_files`) != 0 || f.count(t, `SELECT COUNT(*) FROM episodes WHERE has_file=true AND id IN ('1','2')`) != 0 {
		t.Fatal("episode removals were not reconciled")
	}
	list, err := f.s.List(ctx, "series", "title", nil)
	if err != nil || len(list.Files) != 3 {
		t.Fatalf("custom show folder extras lost after last tracked file deletion: %+v %v", list, err)
	}
}

func TestFulfilledRequestsReconcileWithoutLosingHistory(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "movie")
	f.exec(t, `INSERT INTO media_requests(id,media_type,tmdb_id,media_id,title,status,fulfilled_at)
		VALUES('request','movie','123','title','Movie','available','2026-01-01T00:00:00Z')`)
	store := requests.NewStore(f.db)
	f.s.reconcile = func(ctx context.Context) error { return store.ReconcileAvailability(ctx, time.Now().UTC()) }
	if err := f.s.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.count(t, `SELECT COUNT(*) FROM media_requests WHERE availability='present'`) != 1 {
		t.Fatal("request did not start with present availability")
	}
	if _, err := f.s.Delete(context.Background(), "movie", "title", nil, []string{"movie.mkv"}, "permanent"); err != nil {
		t.Fatal(err)
	}
	if f.count(t, `SELECT COUNT(*) FROM media_requests WHERE status='available' AND availability='missing' AND fulfilled_at='2026-01-01T00:00:00Z'`) != 1 {
		t.Fatal("fulfilled request history or missing availability incorrect")
	}
}

func TestRejectUntrustedFoldersAndChangedPolicy(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "movie")
	if _, err := f.s.Delete(context.Background(), "movie", "title", nil, []string{"movie.mkv"}, "recycle"); !errors.Is(err, errConflict) {
		t.Fatalf("mode mismatch: %v", err)
	}
	f.exec(t, `INSERT INTO movies(id,title,library_id) VALUES('other','Other','lib')`)
	f.exec(t, `INSERT INTO movie_files(id,movie_id,file_path) VALUES('other-file','other',?)`, filepath.Join(f.root, "other.mkv"))
	if _, err := f.s.List(context.Background(), "movie", "title", nil); !errors.Is(err, errConflict) {
		t.Fatalf("shared folder accepted: %v", err)
	}
	f.exec(t, `DELETE FROM movie_files WHERE id='other-file'`)
	original := f.root + "-original"
	if err := os.Rename(f.root, original); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(original, f.root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.List(context.Background(), "movie", "title", nil); !errors.Is(err, errConflict) {
		t.Fatalf("symlink title folder accepted: %v", err)
	}
	f.exec(t, `UPDATE movie_files SET file_path=? WHERE id='video'`, filepath.Join(f.library, "loose.mkv"))
	if _, err := f.s.List(context.Background(), "movie", "title", nil); !errors.Is(err, errConflict) {
		t.Fatalf("loose shared-root file accepted: %v", err)
	}
	f.exec(t, `UPDATE movie_files SET file_path=? WHERE id='video'`, filepath.Join(t.TempDir(), "escape", "movie.mkv"))
	if _, err := f.s.List(context.Background(), "movie", "title", nil); !errors.Is(err, errConflict) {
		t.Fatalf("outside-library file accepted: %v", err)
	}
}

func TestFolderFallbackRequiresExactTitleAndYear(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "movie")
	f.exec(t, `DELETE FROM movie_files`)
	f.exec(t, `UPDATE movies SET year=2024 WHERE id='title'`)
	wrong := filepath.Join(f.library, "Movie (1999)")
	if err := os.Mkdir(wrong, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.List(context.Background(), "movie", "title", nil); !errors.Is(err, errNotFound) {
		t.Fatalf("wrong-year title folder accepted: %v", err)
	}
	correct := filepath.Join(f.library, "Movie (2024)")
	if err := os.Mkdir(correct, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(correct, "movie.nfo"), []byte("extra"), 0600); err != nil {
		t.Fatal(err)
	}
	list, err := f.s.List(context.Background(), "movie", "title", nil)
	if err != nil || len(list.Files) != 1 || list.Files[0].Path != "movie.nfo" {
		t.Fatalf("conventional folder listing: %+v %v", list, err)
	}
	if err := os.Mkdir(filepath.Join(f.library, "Movie"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.List(context.Background(), "movie", "title", nil); !errors.Is(err, errConflict) {
		t.Fatalf("ambiguous title folders accepted: %v", err)
	}
}

func TestDeleteFailuresAreExplicitAndAuditedPartials(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "movie")
	f.s.cfg.RecycleBin = appconfig.RecycleBinConfig{Enabled: true, Path: t.TempDir()}
	f.s.recycle = func(_ context.Context, path, _ string) error {
		if strings.HasSuffix(path, ".nfo") {
			return errors.New("recycle disk full")
		}
		return os.Remove(path)
	}
	deleted, err := f.s.Delete(context.Background(), "movie", "title", nil, []string{"movie.mkv", "movie.nfo"}, "recycle")
	if err == nil || !strings.Contains(err.Error(), "disk full") || len(deleted) != 1 {
		t.Fatalf("partial result: %v %v", deleted, err)
	}
	if f.count(t, `SELECT COUNT(*) FROM audit_log WHERE event_type='file_deleted'`) != 1 ||
		f.count(t, `SELECT COUNT(*) FROM movies WHERE status='missing'`) != 1 {
		t.Fatal("successful part not reconciled/audited")
	}
	if _, err := os.Stat(filepath.Join(f.root, "movie.nfo")); err != nil {
		t.Fatal("failed file should remain")
	}
}

func TestRouterConfirmationAndAdminBoundary(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "movie")
	admin := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Test-Role") != "admin" {
				w.WriteHeader(http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
	router := Router(f.s, admin)
	body := `{"paths":["movie.mkv"],"deletionMode":"permanent"}`
	for _, tc := range []struct {
		method, url, role, confirm, body string
		status                           int
	}{
		{"GET", "/movie/title", "user", "", "", 403},
		{"POST", "/movie/title/delete", "user", "true", body, 403},
		{"POST", "/movie/title/delete", "admin", "", body, 400},
		{"POST", "/movie/title/delete", "admin", "true", body + `{}`, 400},
		{"POST", "/movie/title/delete", "admin", "true", `{"paths":["movie.mkv"],"deletionMode":"permanent","root":"/"}`, 400},
		{"GET", "/series/title?season=bogus", "admin", "", "", 400},
		{"GET", "/movie/title?season=1", "admin", "", "", 400},
		{"GET", "/movie/missing", "admin", "", "", 404},
		{"GET", "/movie/title", "admin", "", "", 200},
		{"POST", "/movie/title/delete", "admin", "true", body, 200},
	} {
		req := httptest.NewRequest(tc.method, tc.url, strings.NewReader(tc.body))
		req.Header.Set("Test-Role", tc.role)
		req.Header.Set("X-Confirm-Delete", tc.confirm)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("%s %s got %d: %s, want %d", tc.method, tc.url, rec.Code, rec.Body, tc.status)
		}
		if tc.status == 200 && tc.method == "POST" {
			var result struct{ Deleted []string }
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || len(result.Deleted) != 1 {
				t.Fatalf("response: %s %v", rec.Body, err)
			}
		}
	}
}

func TestRouterWithRealSessionAuthorization(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "movie")
	store, err := auth.StoreFromDB(f.storage)
	if err != nil {
		t.Fatal(err)
	}
	service, err := auth.NewService(auth.ServiceOptions{
		Store: store, AppConfig: &appconfig.Config{}, AppConfigPath: filepath.Join(t.TempDir(), "config.json"),
		SessionSecret: []byte("title-files-test-session-secret-32bytes"),
	})
	if err != nil {
		t.Fatal(err)
	}
	router := Router(f.s, service.RequireRole("admin"))
	for _, role := range []string{"", "user", "admin"} {
		cookie := ""
		if role != "" {
			user, err := store.CreateUser(context.Background(), auth.CreateUserParams{Username: role, PasswordHash: "unused-session-only", Role: role})
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			cookie, err = auth.SignSession(service.SessionSecret(), auth.SessionPayload{
				UID: user.ID, IAT: now.Unix(), EXP: now.Add(time.Hour).Unix(),
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			url := "/movie/title"
			if method == http.MethodPost {
				url += "/delete"
			}
			req := httptest.NewRequest(method, url, strings.NewReader(`{"paths":["movie.nfo"],"deletionMode":"permanent"}`))
			req.Header.Set("X-Confirm-Delete", "true")
			if cookie != "" {
				req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: cookie})
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			want := map[string]int{"": http.StatusUnauthorized, "user": http.StatusForbidden, "admin": http.StatusOK}[role]
			if rec.Code != want {
				t.Fatalf("%s role %q: got %d %s, want %d", method, role, rec.Code, rec.Body, want)
			}
		}
	}
}
