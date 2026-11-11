package imports

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/movies"
)

func replacementFixture(t *testing.T, incoming, existing string, contents []byte) (*ImportPipeline, string, string, *MatchResult) {
	t.Helper()
	p, svc, destDir := newCollisionPipeline(t, ImportModeCopy)
	p.decisions = NewDecisionMaker(NewUpgradeSpec(&stubProfileStore{profile: anyProfile()}))
	svc.movie.QualityProfileID = "prof-1"
	source := filepath.Join(t.TempDir(), incoming)
	dest := filepath.Join(destDir, "Safety Film (2026).mkv")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("old media"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc.existing = []*movies.MovieFile{{MovieID: "one", FilePath: dest, Quality: existing}}
	if _, err := p.db.Exec(`INSERT INTO movie_files(movie_id, file_path, size, quality) VALUES(?, ?, ?, ?)`, "one", dest, 9, existing); err != nil {
		t.Fatal(err)
	}
	match, err := p.matcher.MatchExact(context.Background(), MediaTypeMovie, "one")
	if err != nil {
		t.Fatal(err)
	}
	return p, source, dest, match
}

func TestMatchedReplacementPolicy(t *testing.T) {
	for _, tt := range []struct {
		name, incoming, existing string
		recycle, reject          bool
	}{
		{"equal without recycler", "Movie.1080p.HDTV.mkv", "hdtv-1080p", false, false},
		{"upgrade with recycler", "Movie.1080p.BluRay.mkv", "hdtv-1080p", true, false},
		{"downgrade", "Movie.1080p.HDTV.mkv", "bluray-1080p", false, true},
		{"profile disallowed", "Movie.720p.HDTV.mkv", "hdtv-1080p", false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, source, dest, match := replacementFixture(t, tt.incoming, tt.existing, []byte("new media"))
			if tt.recycle {
				p.recycler = &RecycleBin{Enabled: true, Path: t.TempDir()}
			}
			_, err := p.importMatchedWith(context.Background(), &downloads.DownloadCompletedEvent{Title: "Movie"}, match, source, filepath.Base(dest), true)
			if (err != nil) != tt.reject {
				t.Fatalf("unexpected import error: %v", err)
			}
			want := "new media"
			if tt.reject {
				want = "old media"
			}
			if data, err := os.ReadFile(dest); err != nil || string(data) != want {
				t.Fatalf("destination: %q, %v", data, err)
			}
			if data, err := os.ReadFile(source); err != nil || string(data) != "new media" {
				t.Fatalf("source: %q, %v", data, err)
			}
			if tt.recycle {
				old := filepath.Join(p.recycler.Path, filepath.Base(filepath.Dir(match.DestPath)), filepath.Base(match.DestPath), filepath.Base(dest))
				if data, err := os.ReadFile(old); err != nil || string(data) != "old media" {
					t.Fatalf("recycled file: %q, %v", data, err)
				}
			}
			if !tt.reject {
				var quality string
				if err := p.db.QueryRow(`SELECT quality FROM movie_files`).Scan(&quality); err != nil || quality != releaseQuality(source) {
					t.Fatalf("replacement quality not registered: %q, %v", quality, err)
				}
			}
		})
	}
}

func TestReplacementFailuresPreserveBoth(t *testing.T) {
	for _, failure := range []string{"verification", "registration", "transfer"} {
		t.Run(failure, func(t *testing.T) {
			data := []byte("new media")
			if failure == "verification" {
				data = nil
			}
			p, source, dest, match := replacementFixture(t, "Movie.1080p.HDTV.mkv", "hdtv-1080p", data)
			if failure == "registration" {
				if _, err := p.db.Exec(`CREATE TRIGGER fail_update BEFORE UPDATE ON movie_files BEGIN SELECT RAISE(FAIL, 'registration failed'); END`); err != nil {
					t.Fatal(err)
				}
			}
			if failure == "transfer" {
				p.importMode = ImportMode("invalid")
				// A symlink source is rejected without touching the old file.
				if err := os.Remove(source); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(t.TempDir(), "target")
				if err := os.WriteFile(target, data, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, source); err != nil {
					t.Fatal(err)
				}
			}
			_, err := p.importMatchedWith(context.Background(), &downloads.DownloadCompletedEvent{Title: "Movie"}, match, source, filepath.Base(dest), true)
			if err == nil {
				t.Fatal("expected failure")
			}
			if got, err := os.ReadFile(dest); err != nil || string(got) != "old media" {
				t.Fatalf("old file not preserved: %q %v", got, err)
			}
			if got, err := os.ReadFile(source); err != nil || string(got) != string(data) {
				t.Fatalf("source not preserved: %q %v", got, err)
			}
		})
	}
}

func TestReplacementRejectsOtherOwnerEvenWhenContentMatches(t *testing.T) {
	p, source, dest, match := replacementFixture(t, "Movie.1080p.HDTV.mkv", "hdtv-1080p", []byte("old media"))
	if _, err := p.db.Exec(`INSERT INTO episode_files (episode_id, file_path) VALUES (?, ?)`, "other", dest); err != nil {
		t.Fatal(err)
	}
	_, err := p.importMatchedWith(context.Background(), &downloads.DownloadCompletedEvent{Title: "Movie"}, match, source, filepath.Base(dest), true)
	if !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("expected other-owner collision, got %v", err)
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != "old media" {
		t.Fatalf("other owner's file changed: %q, %v", got, err)
	}
}

func TestReplacementRecycleFailureLeavesRecoverableBackup(t *testing.T) {
	p, source, dest, match := replacementFixture(t, "Movie.1080p.HDTV.mkv", "hdtv-1080p", []byte("new media"))
	bin := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(bin, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	p.recycler = &RecycleBin{Enabled: true, Path: bin}
	_, err := p.importMatchedWith(context.Background(), &downloads.DownloadCompletedEvent{Title: "Movie"}, match, source, filepath.Base(dest), true)
	if err == nil || !strings.Contains(err.Error(), "preserved at") {
		t.Fatalf("expected recoverable recycle failure, got %v", err)
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(dest), ".loom-replacement-*", "previous"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("missing backup: %v, %v", backups, err)
	}
	if data, err := os.ReadFile(backups[0]); err != nil || string(data) != "old media" {
		t.Fatalf("backup: %q, %v", data, err)
	}
	for path, want := range map[string]string{source: "new media", dest: "new media"} {
		if data, err := os.ReadFile(path); err != nil || string(data) != want {
			t.Fatalf("file %s: %q, %v", path, data, err)
		}
	}
	if _, err := p.importMatchedWith(context.Background(), &downloads.DownloadCompletedEvent{Title: "Movie"}, match, source, filepath.Base(dest), true); err != nil {
		t.Fatalf("retry failed to finish registration: %v", err)
	}
}

func TestReplacementSerializesConcurrentImports(t *testing.T) {
	p, source, dest, match := replacementFixture(t, "Movie.1080p.HDTV.mkv", "hdtv-1080p", []byte("new media"))
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := p.importMatchedWith(context.Background(), &downloads.DownloadCompletedEvent{Title: "Movie"}, match, source, filepath.Base(dest), true)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !errors.Is(err, ErrDestinationExists) {
			t.Fatalf("concurrent import: %v", err)
		}
	}
	if got, err := os.ReadFile(dest); err != nil || strings.TrimSpace(string(got)) != "new media" {
		t.Fatalf("destination: %q, %v", got, err)
	}
}

func TestReplacementKeepsLivePathAvailable(t *testing.T) {
	p, source, dest, match := replacementFixture(t, "Movie.1080p.HDTV.mkv", "hdtv-1080p", []byte("new media"))
	alternate := filepath.Join(t.TempDir(), filepath.Base(source))
	if err := os.WriteFile(alternate, []byte("old media"), 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	observed := make(chan error, 1)
	go func() {
		for {
			select {
			case <-done:
				observed <- nil
				return
			default:
				data, err := os.ReadFile(dest)
				if err != nil {
					observed <- err
					return
				}
				if string(data) != "new media" && string(data) != "old media" {
					observed <- errors.New("live destination contained incomplete media")
					return
				}
			}
		}
	}()
	defer func() {
		close(done)
		if err := <-observed; err != nil {
			t.Errorf("live destination was unavailable during replacement: %v", err)
		}
	}()
	for i := 0; i < 20; i++ {
		path := source
		if i%2 == 1 {
			path = alternate
		}
		if _, err := p.importMatchedWith(context.Background(), &downloads.DownloadCompletedEvent{Title: "Movie"}, match, path, filepath.Base(dest), true); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEpisodeReplacementAndManualReimportEnforceProfile(t *testing.T) {
	p, _, movieDir := newCollisionPipeline(t, ImportModeCopy)
	p.matcher.seriesSvc = &transferSeriesService{profile: "prof-1"}
	p.decisions = NewDecisionMaker(NewUpgradeSpec(&stubProfileStore{profile: anyProfile()}))
	destDir := filepath.Join(filepath.Dir(movieDir), "Test Show", "Season 02")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "Test.Show.1080p.HDTV.mkv"
	dest := filepath.Join(destDir, name)
	source := filepath.Join(t.TempDir(), name)
	for path, content := range map[string]string{dest: "old episode", source: "new episode"} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.db.Exec(`INSERT INTO episode_files VALUES (?, ?, ?, ?, ?)`, "episode", dest, 11, "hdtv-1080p", "2026"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ReimportFile(context.Background(), MediaTypeEpisode, "episode", source, ReimportOptions{}); err != nil {
		t.Fatalf("equal quality episode reimport: %v", err)
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != "new episode" {
		t.Fatalf("episode replacement: %q, %v", got, err)
	}
	var count int
	if err := p.db.QueryRow(`SELECT COUNT(*) FROM episode_files`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("episode file records: %d, %v", count, err)
	}
	// Explicit replace_always and manual matching must not override a profile.
	disallowed := filepath.Join(t.TempDir(), "Test.Show.720p.HDTV.mkv")
	if err := os.WriteFile(disallowed, []byte("lower quality"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ReimportFile(context.Background(), MediaTypeEpisode, "episode", disallowed, ReimportOptions{ConflictPolicy: ConflictReplaceAlways}); err == nil {
		t.Fatal("explicit reimport accepted disallowed quality")
	}
	if _, err := p.ImportManualMatch(context.Background(), disallowed, MediaTypeEpisode, "episode"); err == nil {
		t.Fatal("manual match accepted disallowed quality")
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != "new episode" {
		t.Fatalf("rejected import changed episode: %q, %v", got, err)
	}
}
