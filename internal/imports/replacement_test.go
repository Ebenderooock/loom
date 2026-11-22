package imports

import (
	"context"
	"errors"
	"github.com/ebenderooock/loom/internal/safety"
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
	backups, err := replacementBackups(dest)
	if err != nil || len(backups) != 1 {
		t.Fatalf("missing backup: %v, %v", backups, err)
	}
	if data, err := os.ReadFile(backups[0]); err != nil || string(data) != "old media" {
		t.Fatalf("backup: %q, %v", data, err)
	}
	saved, err := os.Stat(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(backups[0]))
	if err != nil {
		t.Fatal(err)
	}
	originals := 0
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if os.SameFile(saved, info) {
			originals++
		}
	}
	if originals != 1 {
		t.Fatalf("expected one preserved original without redundant hardlink aliases, got %d", originals)
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

func replacementBackups(dest string) ([]string, error) {
	name := "previous"
	if replacementSwapsFiles {
		name = "incoming"
	}
	return filepath.Glob(filepath.Join(filepath.Dir(dest), ".loom-replacement-*", name))
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
	t.Run("success", func(t *testing.T) {
		testReplacementKeepsLivePathAvailable(t, false)
	})
	t.Run("registration rollback", func(t *testing.T) {
		testReplacementKeepsLivePathAvailable(t, true)
	})
}

func testReplacementKeepsLivePathAvailable(t *testing.T, failRegistration bool) {
	t.Helper()
	p, source, dest, match := replacementFixture(t, "Movie.1080p.HDTV.mkv", "hdtv-1080p", []byte("new media"))
	if failRegistration {
		if _, err := p.db.Exec(`CREATE TRIGGER fail_update BEFORE UPDATE ON movie_files BEGIN SELECT RAISE(FAIL, 'registration failed'); END`); err != nil {
			t.Fatal(err)
		}
	}
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
				data, err := readReplacementDestination(dest)
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
	stopReader := sync.OnceFunc(func() {
		close(done)
		if err := <-observed; err != nil {
			t.Errorf("live destination was unavailable during replacement: %v", err)
		}
	})
	defer stopReader()
	for i := 0; i < 20; i++ {
		path := source
		wantSource := "new media"
		if i%2 == 1 && !failRegistration {
			path = alternate
			wantSource = "old media"
		}
		before, err := os.Stat(dest)
		if err != nil {
			t.Fatal(err)
		}
		previous, err := readReplacementDestination(dest)
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.importMatchedWith(context.Background(), &downloads.DownloadCompletedEvent{Title: "Movie"}, match, path, filepath.Base(dest), true)
		if failRegistration && err == nil {
			t.Fatal("expected registration failure")
		}
		if data, err := os.ReadFile(path); err != nil || string(data) != wantSource {
			t.Fatalf("replacement changed source bytes: %q, %v", data, err)
		}
		if failRegistration && replacementBlockedByOpenReader(err, dest, "previous") {
			// A Windows reader can open the new file before rollback begins.
			stopReader()
			backups, globErr := replacementBackups(dest)
			if globErr != nil || len(backups) != 1 {
				t.Fatalf("blocked rollback lost backup: %v, %v", backups, globErr)
			}
			if !strings.Contains(err.Error(), backups[0]) {
				t.Fatalf("blocked rollback did not report backup path: %v", err)
			}
			if data, err := os.ReadFile(backups[0]); err != nil || string(data) != string(previous) {
				t.Fatalf("blocked rollback lost old bytes: %q, %v", data, err)
			}
			if data, err := os.ReadFile(dest); err != nil || string(data) != wantSource {
				t.Fatalf("blocked rollback changed new bytes: %q, %v", data, err)
			}
			var quality string
			var size int64
			if err := p.db.QueryRow(`SELECT quality, size FROM movie_files`).Scan(&quality, &size); err != nil || quality != "hdtv-1080p" || size != 9 {
				t.Fatalf("blocked rollback changed registration: quality=%q size=%d err=%v", quality, size, err)
			}
			break
		}
		if err != nil {
			rolledBack := failRegistration && strings.Contains(err.Error(), "library update failed (source preserved)")
			if !rolledBack && !replacementBlockedByOpenReader(err, dest, "incoming") {
				t.Fatal(err)
			}
			current, statErr := os.Stat(dest)
			if statErr != nil || !os.SameFile(before, current) {
				t.Fatalf("blocked replacement changed the live file: %v", statErr)
			}
			if data, err := readReplacementDestination(dest); err != nil || string(data) != string(previous) {
				t.Fatalf("blocked replacement changed live bytes: %q, %v", data, err)
			}
		}
	}
	stopReader()
	if failRegistration {
		if _, err := p.db.Exec(`DROP TRIGGER fail_update`); err != nil {
			t.Fatal(err)
		}
	}
	// Force a genuine replacement after closing the reader, even if Windows
	// rejected every concurrent attempt. A permanent publication failure fails.
	const final = "final complete media"
	if err := os.WriteFile(alternate, []byte(final), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.importMatchedWith(context.Background(), &downloads.DownloadCompletedEvent{Title: "Movie"}, match, alternate, filepath.Base(dest), true); err != nil {
		t.Fatalf("replacement after reader stopped: %v", err)
	}
	for _, path := range []string{alternate, dest} {
		if data, err := os.ReadFile(path); err != nil || string(data) != final {
			t.Fatalf("final replacement at %s: %q, %v", path, data, err)
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

func TestDowngradeWaitsForReviewApproval(t *testing.T) {
	ctx := context.Background()
	p, _, movieDir := newCollisionPipeline(t, ImportModeCopy)
	applyReviewMigrations(t, p.db)
	store := safety.NewReviewStore(p.db)
	p.reviewStore = store
	seriesSvc := &transferSeriesService{profile: "prof-1"}
	p.matcher.seriesSvc = seriesSvc
	p.decisions = NewDecisionMaker(NewUpgradeSpec(&stubProfileStore{profile: anyProfile()}))
	destDir := filepath.Join(filepath.Dir(movieDir), "Test Show", "Season 02")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(destDir, "Test.Show.1080p.HDTV.mkv")
	if err := os.WriteFile(dest, []byte("better episode"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.db.Exec(`INSERT INTO episode_files VALUES (?, ?, ?, ?, ?)`, "episode", dest, 14, "hdtv-1080p", "2026"); err != nil {
		t.Fatal(err)
	}
	var resumed []*safety.Review
	store.OnApproved(func(_ context.Context, r *safety.Review) { resumed = append(resumed, r) })
	lower := func(name string) string {
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte("lower quality"), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	held := func(source string) *safety.Review {
		t.Helper()
		for range 2 { // a repeated import must not duplicate the review
			if _, err := p.ReimportFile(ctx, MediaTypeEpisode, "episode", source, ReimportOptions{}); err == nil ||
				!strings.Contains(err.Error(), "pending review") {
				t.Fatalf("downgrade was not held for review: %v", err)
			}
		}
		pending, err := store.ListPending(ctx)
		if err != nil || len(pending) != 1 || pending[0].Kind != safety.ReviewKindQualityDowngrade || pending[0].DownloadPath != source {
			t.Fatalf("pending reviews: %+v, %v", pending, err)
		}
		if got, _ := os.ReadFile(dest); string(got) != "better episode" {
			t.Fatalf("held downgrade replaced the library file: %q", got)
		}
		return pending[0]
	}

	declined := lower("Test.Show.720p.HDTV.mkv")
	if err := store.Reject(ctx, held(declined).ID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ReimportFile(ctx, MediaTypeEpisode, "episode", declined, ReimportOptions{}); err == nil ||
		!strings.Contains(err.Error(), "replacement declined") {
		t.Fatalf("declined downgrade: %v", err)
	}

	approved := lower("Test.Show.720p.HDTV.mkv")
	review := held(approved)
	if err := store.Approve(ctx, review.ID); err != nil {
		t.Fatal(err)
	}
	if len(resumed) != 1 || resumed[0].ID != review.ID || resumed[0].Kind != safety.ReviewKindQualityDowngrade {
		t.Fatalf("approval hook: %+v", resumed)
	}
	if _, err := p.ReimportFile(ctx, MediaTypeEpisode, "episode", approved, ReimportOptions{}); err != nil {
		t.Fatalf("approved downgrade: %v", err)
	}
	if n := len(seriesSvc.files); n != 1 || seriesSvc.files[0].Quality != "hdtv-720p" {
		t.Fatalf("approved downgrade was not registered: %+v", seriesSvc.files)
	}
}

func TestResumeApprovedReviewRetriesOnlyDowngradeHolds(t *testing.T) {
	f, _ := newSafetyPipeline(t)
	var retried []string
	retry := func(id string) { retried = append(retried, id) }
	f.p.ResumeApprovedReview(context.Background(), &safety.Review{WorkflowID: "safety-hold"}, retry)
	f.p.ResumeApprovedReview(context.Background(), &safety.Review{Kind: safety.ReviewKindQualityDowngrade, WorkflowID: "wf"}, retry)
	if len(retried) != 1 || retried[0] != "wf" {
		t.Fatalf("retried workflows: %v", retried)
	}
}

func TestGrabDowngradeWarning(t *testing.T) {
	p, _, movieDir := newCollisionPipeline(t, ImportModeCopy)
	p.matcher.seriesSvc = &transferSeriesService{profile: "prof-1"}
	p.upgradeSpec = NewUpgradeSpec(&stubProfileStore{profile: anyProfile()})
	dest := filepath.Join(movieDir, "Test.Show.1080p.HDTV.mkv")
	if _, err := p.db.Exec(`INSERT INTO episode_files VALUES (?, ?, ?, ?, ?)`, "episode", dest, 14, "hdtv-1080p", "2026"); err != nil {
		t.Fatal(err)
	}
	for title, warned := range map[string]bool{
		"Test.Show.S02E01.720p.HDTV-GRP":    true,
		"Test.Show.S02E01.1080p.BluRay-GRP": false,
		"Test.Show.S02E01.2160p.WEB-DL-GRP": false,
	} {
		if got := p.GrabDowngradeWarning(context.Background(), "episode", []string{"episode"}, title); (got != "") != warned {
			t.Errorf("%s: warning %q, want warned=%v", title, got, warned)
		}
	}
}
