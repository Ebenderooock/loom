package imports

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/kernel/eventbus"
	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/series"
	"github.com/ebenderooock/loom/internal/workflows"
)

func TestProcessImportBareEpisodePack(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	s := dragonBallCatalog(f)
	profile := unrestrictedProfile(t)
	for _, show := range s.shows {
		show.QualityProfileID = profile.ID
	}
	f.p.decisions = NewDecisionMaker(&SampleSpec{}, &FreeSpaceSpec{}, &UnpackingSpec{}, &DangerousFileSpec{}, NewUpgradeSpec(&stubProfileStore{profile: profile}))
	s.seasons[1].EpisodeCount = 153
	s.seasons = append(s.seasons[:2], s.seasons[3:]...)
	fixture, err := os.ReadFile("testdata/dragon_ball_episode_pack.txt")
	if err != nil {
		t.Fatal(err)
	}
	names := strings.Split(strings.TrimSpace(string(fixture)), "\n")
	if len(names) != 153 {
		t.Fatalf("fixture has %d filenames, want 153", len(names))
	}
	for i, ep := range s.episodes[2:] {
		ep.SeasonID, ep.EpisodeNumber = "db-s1", i+1
		_, ep.Title, _ = bareEpisode(names[i])
		switch i + 1 {
		case 75:
			ep.Title = "The Strong Ones"
		case 82:
			ep.Title = "The Rampage of InoShikaCho"
		}
	}
	root := filepath.Join(t.TempDir(), "Dragon Ball The Complete Series (1995\u20132003)")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := f.p.processImport(context.Background(), &downloads.DownloadCompletedEvent{
		Title: "Dragon Ball The Complete Series (1995\u20132003)", DownloadID: "pack",
	}, root)
	if err != nil || len(paths) != 153 {
		t.Fatalf("full pack import: %d paths, %v", len(paths), err)
	}
	files := f.svc.episodeFiles()
	if len(files) != 153 {
		t.Fatalf("pack mapped onto %d distinct episodes, want 153", len(files))
	}
	for i := 1; i <= 153; i++ {
		path := files[fmt.Sprintf("db%d", i)]
		if path == "" {
			t.Fatalf("episode %d was skipped", i)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want := names[i-1]
		if string(content) != want {
			t.Fatalf("episode %d received wrong file: %q, want %q", i, content, want)
		}
	}
}

func TestImportUsesOnlyMatchingReleaseQualityContext(t *testing.T) {
	title := "Dragon.Ball.Super.Beerus.S01E01.CR.1080p.WEBRip.10bits.x265-Rapta"
	for _, tc := range []struct {
		name, file, title string
		allowed           bool
	}{
		{"quality omitted", "Dragon Ball Super - Beerus - S01E01 - The God Who Destroys.mkv", title, true},
		{"explicit lower quality", "Dragon.Ball.Super.Beerus.S01E01.720p.HDTV.mkv", title, false},
		{"other series", "Dragon Ball Super - Beerus - S01E01 - The God Who Destroys.mkv", "Dragon.Ball.Z.S01E01.1080p.WEBRip", false},
		{"other episode", "Dragon Ball Super - Beerus - S01E01 - The God Who Destroys.mkv", "Dragon.Ball.Super.Beerus.S01E02.1080p.WEBRip", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPackPipeline(t, ImportModeCopy, nil)
			profile := anyProfile()
			f.svc.show.Title, f.svc.show.QualityProfileID = "Dragon Ball Super - Beerus", profile.ID
			f.p.decisions = NewDecisionMaker(&SampleSpec{}, &FreeSpaceSpec{}, &UnpackingSpec{}, &DangerousFileSpec{}, NewUpgradeSpec(&stubProfileStore{profile: profile}))
			root := t.TempDir()
			source := filepath.Join(root, tc.file)
			if err := os.WriteFile(source, []byte("episode"), 0o600); err != nil {
				t.Fatal(err)
			}
			paths, err := f.p.processImport(context.Background(), &downloads.DownloadCompletedEvent{DownloadID: "pack", Title: tc.title}, root)
			if (err == nil) != tc.allowed {
				t.Fatalf("release-quality import: %v, %v; allow=%v", paths, err, tc.allowed)
			}
			if tc.allowed && (len(f.svc.files) != 1 || f.svc.files[0].Quality != "webrip-1080p") {
				t.Fatalf("release quality was not retained in library metadata: %+v", f.svc.files)
			}
		})
	}
}

func TestManualBareEpisodeKeepsAncestorSeriesContext(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	dragonBallCatalog(f)
	dir := filepath.Join(t.TempDir(), "Dragon Ball", "Season 02")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "Episode 1 Adventure 14.mkv")
	if err := os.WriteFile(source, []byte("season two premiere"), 0o600); err != nil {
		t.Fatal(err)
	}
	scan := f.p.scanSingleFile(context.Background(), source)
	if scan.MatchedMediaID != "db14" || scan.DetectedTitle != "Dragon Ball" ||
		scan.DetectedSeason != 2 || scan.DetectedEpisode != 1 || scan.Confidence != 1 {
		t.Fatalf("scan discarded resolved episode context: %+v", scan)
	}
	dest, err := f.p.importSingleFile(context.Background(), &downloads.DownloadCompletedEvent{Title: filepath.Base(source)}, source)
	if err != nil || filepath.Base(filepath.Dir(dest)) != "Season 02" || f.svc.episodeFiles()["db14"] != dest {
		t.Fatalf("manual import lost scan's ancestor series context: %q, %v", dest, err)
	}
}

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
		JobWorkers:  -1, // tests drive the job queue explicitly
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

func TestVerifyMovieImportYear(t *testing.T) {
	match := &MatchResult{Matched: true, MediaType: MediaTypeMovie, Title: "Street Fighter", Year: 2026}
	for _, tc := range []struct {
		name  string
		title string
		file  string
		fail  bool
	}{
		{"wrong torrent year", "Street Fighter 1994 Open Matte 1080p WEB-DL HEVC x265 5.1 BONE", "/downloads/hash/video.mkv", true},
		{"wrong file year", "Street Fighter 2026 1080p WEB-DL", "/downloads/hash/Street.Fighter.1994.mkv", true},
		{"wrong folder year", "Street Fighter", "/downloads/Street Fighter (1994)/video.mkv", true},
		{"correct year", "Street Fighter 2026 1080p WEB-DL", "/downloads/Street Fighter (2026)/Street.Fighter.2026.mkv", false},
		{"one year earlier", "Street Fighter 2025 1080p WEB-DL", "/downloads/hash/video.mkv", false},
		{"one year later", "Street Fighter 2027 1080p WEB-DL", "/downloads/hash/video.mkv", false},
		{"two year difference", "Street Fighter 2024 1080p WEB-DL", "/downloads/hash/video.mkv", true},
		{"unknown year", "Street Fighter 1080p WEB-DL", "/downloads/hash/video.mkv", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyMovieImportYear(match, tc.title, tc.file)
			if (err != nil) != tc.fail {
				t.Fatalf("want failure %v, got %v", tc.fail, err)
			}
		})
	}
	episode := &MatchResult{Matched: true, MediaType: MediaTypeEpisode, Title: "Lanterns", Year: 2026}
	if err := verifyMovieImportYear(episode, "Lanterns 2027 S02E01 1080p WEB-DL", "/downloads/video.mkv"); err != nil {
		t.Fatalf("episode release year must not be compared with series premiere year: %v", err)
	}
}

func TestImportSingleFile_WorkflowCannotOverrideWrongMovieYear(t *testing.T) {
	ctx := context.Background()
	opts := basePipelineOptions(t)
	root := t.TempDir()
	source := filepath.Join(root, "video.mkv")
	destDir := filepath.Join(root, "Street Fighter (2026)")
	dest := filepath.Join(destDir, "Street Fighter (2026).mkv")
	if err := os.WriteFile(source, []byte("wrong movie"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("existing movie"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := opts.DB.Exec(`CREATE TABLE libraries (
		id TEXT PRIMARY KEY, name TEXT, path TEXT, media_type TEXT,
		monitor_on_add INTEGER, quality_profile_id TEXT,
		unmonitor_on_delete INTEGER, auto_archive_watched INTEGER,
		auto_archive_days_after_watch INTEGER, created_at DATETIME, updated_at DATETIME
	)`)
	if err != nil {
		t.Fatal(err)
	}
	opts.LibStore = libraries.NewStore(opts.DB)
	if err := opts.LibStore.Create(ctx, &libraries.Library{ID: "lib", Name: "Movies", Path: root, MediaType: "movie"}); err != nil {
		t.Fatal(err)
	}
	opts.MoviesSvc = &stubMoviesSvc{moviesByID: map[string]*movies.Movie{
		"remake": {ID: "remake", Title: "Street Fighter", Year: 2026, LibraryID: "lib"},
	}}
	store, err := workflows.NewStore(opts.DB)
	if err != nil {
		t.Fatal(err)
	}
	title := "Street Fighter 1994 Open Matte 1080p WEB-DL HEVC x265 5.1 BONE"
	if err := store.Create(ctx, &workflows.Workflow{
		Type: workflows.TypeMovieSearch, State: workflows.StateImporting, MediaType: workflows.MediaTypeMovie,
		GrabTitle: title, DownloadClientID: "client", DownloadID: "download",
		Items: []workflows.Item{{MediaType: workflows.MediaTypeMovie, MediaID: "remake"}},
	}); err != nil {
		t.Fatal(err)
	}
	opts.WorkflowEngine = workflows.NewEngine(store, nil, opts.Logger)
	p, err := NewPipeline(opts)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.importSingleFile(ctx, &downloads.DownloadCompletedEvent{
		ClientID: "client", DownloadID: "download", Title: title,
	}, source)
	if err == nil || !strings.Contains(err.Error(), "release year 1994") {
		t.Fatalf("workflow-linked wrong movie must fail import: %v", err)
	}
	for path, want := range map[string]string{source: "wrong movie", dest: "existing movie"} {
		content, err := os.ReadFile(path)
		if err != nil || string(content) != want {
			t.Fatalf("rejected import changed %q: %q, %v", path, content, err)
		}
	}
	var status, mediaID string
	if err := opts.DB.QueryRow(`SELECT status, media_id FROM import_history LIMIT 1`).Scan(&status, &mediaID); err != nil {
		t.Fatal(err)
	}
	if status != string(StatusFailed) || mediaID != "remake" {
		t.Fatalf("expected recorded failure for remake, got %q %q", status, mediaID)
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
	_, err = p.processImport(context.Background(), &downloads.DownloadCompletedEvent{Title: "Missing"}, missingPath)
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
