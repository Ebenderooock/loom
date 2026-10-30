package requests

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func execAvailabilitySQL(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func approvedRequest(t *testing.T, store *Store, mediaType MediaType, mediaID string) Request {
	t.Helper()
	ctx := context.Background()
	r, err := store.Create(ctx, Request{UserID: "1", MediaType: mediaType, TMDBID: mediaID, Title: mediaID})
	if err != nil {
		t.Fatal(err)
	}
	won, err := store.ClaimForApproval(ctx, r.ID, "admin")
	if err != nil || !won {
		t.Fatalf("claim: won=%v err=%v", won, err)
	}
	if err := store.MarkApproved(ctx, r.ID, mediaID); err != nil {
		t.Fatal(err)
	}
	return r
}

func assertAvailability(t *testing.T, store *Store, id string, want Status) {
	t.Helper()
	prepareAvailabilityFiles(context.Background(), t, store.db)
	ctx := context.Background()
	if err := store.ReconcileAvailability(ctx, time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != want {
		t.Fatalf("status=%s, want %s", got.Status, want)
	}
}

// Older reconciliation fixtures describe recorded imports; materialize them
// inside an actual connected library rather than trusting fictional paths.
func prepareAvailabilityFiles(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	var root string
	err := db.QueryRowContext(ctx, `SELECT path FROM libraries WHERE id='availability-test'`).Scan(&root)
	if errors.Is(err, sql.ErrNoRows) {
		root = t.TempDir()
		exec(`INSERT INTO libraries(id,name,path) VALUES('availability-test','Files',?)`, root)
	} else if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"movies", "series", "artists"} {
		exec(`UPDATE ` + table + ` SET library_id='availability-test' WHERE COALESCE(library_id,'')=''`)
	}
	for _, table := range []string{"movie_files", "episode_files", "track_files"} {
		rows, err := db.QueryContext(ctx, `SELECT id,file_path FROM `+table)
		if err != nil {
			t.Fatal(err)
		}
		files := map[string]string{}
		for rows.Next() {
			var id, path string
			if err := rows.Scan(&id, &path); err != nil {
				t.Fatal(err)
			}
			files[id] = path
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		_ = rows.Close()
		for id, path := range files {
			if strings.HasPrefix(path, root+string(filepath.Separator)) {
				continue
			}
			path = filepath.Join(root, filepath.Base(path))
			if err := os.WriteFile(path, []byte("media"), 0600); err != nil {
				t.Fatal(err)
			}
			exec(`UPDATE `+table+` SET file_path=? WHERE id=?`, path, id)
		}
	}
}
func TestReconcileMovieRequiresLiveMatchingFile(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	r := approvedRequest(t, store, MediaMovie, "movie")
	execAvailabilitySQL(t, db, `INSERT INTO movies(id,title) VALUES ('movie','Movie'),('other','Other')`)
	assertAvailability(t, store, r.ID, StatusApproved)
	execAvailabilitySQL(t, db, `INSERT INTO movie_files(id,movie_id,file_path) VALUES ('file','other','/other.mkv')`)
	assertAvailability(t, store, r.ID, StatusApproved)
	execAvailabilitySQL(t, db, `UPDATE movie_files SET movie_id='movie',deleted_at=CURRENT_TIMESTAMP`)
	assertAvailability(t, store, r.ID, StatusApproved)
	execAvailabilitySQL(t, db, `UPDATE movie_files SET deleted_at=NULL`)
	assertAvailability(t, store, r.ID, StatusAvailable)
	assertAvailability(t, store, r.ID, StatusAvailable)
}

func TestReconcileSeriesRequiresAllDueMonitoredEpisodes(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	r := approvedRequest(t, store, MediaSeries, "series")
	execAvailabilitySQL(t, db, `INSERT INTO series(id,title) VALUES ('series','Series')`)
	execAvailabilitySQL(t, db, `INSERT INTO seasons(id,series_id,season_number) VALUES ('season','series',1)`)
	execAvailabilitySQL(t, db, `INSERT INTO episodes(id,series_id,season_id,episode_number,air_date,air_date_utc)
		VALUES ('first','series','season',1,'2026-10-20','2026-10-20T12:00:00Z'),
			('second','series','season',2,'2026-10-21',''),
			('future','series','season',3,'2026-10-22','2026-10-22T23:00:00Z'),
			('unmonitored','series','season',4,'2026-10-01','')`)
	execAvailabilitySQL(t, db, `UPDATE episodes SET monitored=false WHERE id='unmonitored'`)
	assertAvailability(t, store, r.ID, StatusApproved)
	execAvailabilitySQL(t, db, `INSERT INTO episode_files(id,episode_id,series_id,file_path)
		VALUES ('one','first','series','/first.mkv')`)
	assertAvailability(t, store, r.ID, StatusApproved)
	execAvailabilitySQL(t, db, `INSERT INTO episode_files(id,episode_id,series_id,file_path)
		VALUES ('two','second','series','/second.mkv')`)
	assertAvailability(t, store, r.ID, StatusAvailable)
}

func TestReconcileSeriesUnknownDatesAndMissingMetadataRemainApproved(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	r := approvedRequest(t, store, MediaSeries, "series")
	execAvailabilitySQL(t, db, `INSERT INTO series(id,title) VALUES ('series','Series')`)
	assertAvailability(t, store, r.ID, StatusApproved)
	execAvailabilitySQL(t, db, `INSERT INTO seasons(id,series_id,season_number) VALUES ('season','series',1)`)
	execAvailabilitySQL(t, db, `INSERT INTO episodes(id,series_id,season_id,episode_number)
		VALUES ('first','series','season',1),('unknown','series','season',2)`)
	execAvailabilitySQL(t, db, `INSERT INTO episode_files(id,episode_id,series_id,file_path)
		VALUES ('one','first','series','/first.mkv')`)
	assertAvailability(t, store, r.ID, StatusApproved)
	execAvailabilitySQL(t, db, `UPDATE episodes SET monitored=false WHERE id='unknown'`)
	assertAvailability(t, store, r.ID, StatusAvailable)
}

func TestReconcileArtistRequiresCompleteSelectedReleasedAlbums(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	r := approvedRequest(t, store, MediaArtist, "artist")
	execAvailabilitySQL(t, db, `INSERT INTO artists(id,name) VALUES ('artist','Artist')`)
	execAvailabilitySQL(t, db, `INSERT INTO albums(id,artist_id,title,release_date)
		VALUES ('album','artist','Album','2020-01-01'),
			('missing','artist','Missing track list','2020-01-01'),
			('future','artist','Future','2027-01-01')`)
	execAvailabilitySQL(t, db, `INSERT INTO album_artists(album_id,artist_id)
		SELECT id,artist_id FROM albums`)
	execAvailabilitySQL(t, db, `INSERT INTO album_releases(id,album_id) VALUES ('selected','album'),('other','album')`)
	execAvailabilitySQL(t, db, `UPDATE albums SET selected_release_id='selected' WHERE id='album'`)
	execAvailabilitySQL(t, db, `INSERT INTO tracks(id,album_id,release_id,track_number)
		VALUES ('one','album','selected',1),('two','album','selected',2),('alternative','album','other',1)`)
	assertAvailability(t, store, r.ID, StatusApproved)
	execAvailabilitySQL(t, db, `INSERT INTO track_files(id,track_id,file_path)
		VALUES ('first','one','/one.flac'),('alt','alternative','/alternative.flac')`)
	assertAvailability(t, store, r.ID, StatusApproved)
	execAvailabilitySQL(t, db, `INSERT INTO track_files(id,track_id,file_path,deleted_at)
		VALUES ('second','two','/two.flac',CURRENT_TIMESTAMP)`)
	assertAvailability(t, store, r.ID, StatusApproved)
	execAvailabilitySQL(t, db, `UPDATE track_files SET deleted_at=NULL WHERE id='second'`)
	assertAvailability(t, store, r.ID, StatusApproved)
	// An excluded catalogue entry must not hold an otherwise complete request open.
	execAvailabilitySQL(t, db, `UPDATE album_artists SET catalog_included=false WHERE album_id='missing'`)
	assertAvailability(t, store, r.ID, StatusAvailable)
}

func TestReconcileDoesNotChangeUnapprovedRequests(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	execAvailabilitySQL(t, db, `INSERT INTO movies(id,title) VALUES ('movie','Movie')`)
	execAvailabilitySQL(t, db, `INSERT INTO movie_files(id,movie_id,file_path) VALUES ('file','movie','/movie.mkv')`)
	for _, status := range []Status{StatusPending, StatusApproving, StatusRejected, StatusFailed} {
		r, err := store.Create(ctx, Request{UserID: "1", MediaType: MediaMovie, TMDBID: string(status), Title: "Movie"})
		if err != nil {
			t.Fatal(err)
		}
		execAvailabilitySQL(t, db, `UPDATE media_requests SET status=?,media_id='movie' WHERE id=?`, status, r.ID)
		assertAvailability(t, store, r.ID, status)
	}
}

func TestApproveAlreadyImportedBecomesAvailable(t *testing.T) {
	f := &fakeFulfiller{}
	svc := newSvc(t, f, okValidator{})
	ctx := context.Background()
	r, err := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "603"})
	if err != nil {
		t.Fatal(err)
	}

	execAvailabilitySQL(t, svc.store.db, `INSERT INTO movies(id,title) VALUES ('movie','Movie')`)
	execAvailabilitySQL(t, svc.store.db, `INSERT INTO movie_files(id,movie_id,file_path) VALUES ('file','movie','/movie.mkv')`)
	prepareAvailabilityFiles(ctx, t, svc.store.db)
	f.movieExists = map[string]string{"603": "movie"}
	out, err := svc.Approve(ctx, r.ID, "qp", "lib", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != StatusAvailable || out.DecidedBy != "admin" {
		t.Fatalf("request=%+v", out)
	}
}

type importingFulfiller struct {
	fakeFulfiller
	db *sql.DB
	t  *testing.T
}

func (f *importingFulfiller) FulfillMovie(ctx context.Context, _, _, _ string) (string, error) {
	if _, err := f.db.ExecContext(ctx, `INSERT INTO movies(id,title) VALUES ('movie','Movie')`); err != nil {
		return "", err
	}
	_, err := f.db.ExecContext(ctx, `INSERT INTO movie_files(id,movie_id,file_path) VALUES ('file','movie','/movie.mkv')`)
	if err == nil {
		prepareAvailabilityFiles(ctx, f.t, f.db)
	}
	return "movie", err
}

func TestImportCompletingDuringApprovalIsNotLost(t *testing.T) {
	f := &importingFulfiller{t: t}
	svc := newSvc(t, f, okValidator{})
	f.db = svc.store.db
	ctx := context.Background()
	r, err := svc.Create(ctx, "1", "alice", false, CreateInput{MediaType: MediaMovie, TMDBID: "603"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.Approve(ctx, r.ID, "qp", "lib", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != StatusAvailable {
		t.Fatalf("import completed during approval but status=%s", out.Status)
	}
}

func TestReconcileAvailabilitySurfacesDatabaseErrors(t *testing.T) {
	svc := newSvc(t, &fakeFulfiller{}, okValidator{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := svc.ReconcileAvailability(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled reconciliation: %v", err)
	}
}
