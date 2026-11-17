package imports

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/metadata"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/workflows"
)

const encoreFilename = "Avengers.Endgame.Encore.2026.2160p.IMAX.DSNP.WEB-DL.DV.HDR10+.MULTi[Ben The Men].mkv"

type movieReleaseStub struct {
	releases []metadata.MovieRelease
	err      error
}

func (s *movieReleaseStub) GetMovieReleases(_ context.Context, id int) ([]metadata.MovieRelease, error) {
	if id != 299534 {
		return nil, errors.New("unexpected TMDB identity")
	}
	return s.releases, s.err
}

func encorePipeline(t *testing.T) (*ImportPipeline, *collisionMoviesService, *movieReleaseStub, string) {
	t.Helper()
	p, svc, oldDir := newCollisionPipeline(t, ImportModeCopy)
	tmdbID := "299534"
	svc.movie = &movies.Movie{
		ID: "original", Title: "Avengers: Endgame", Year: 2019, TMDBID: &tmdbID, LibraryID: "lib",
	}
	p.matcher.moviesSvc = &literalSearchMoviesService{
		collisionMoviesService: svc, candidates: []*movies.Movie{svc.movie},
	}
	source := &movieReleaseStub{releases: []metadata.MovieRelease{
		{Title: "Avengers Endgame: Encore", ReleaseDate: "2026-09-25"},
	}}
	p.matcher.movieReleases = source
	p.decisions = NewDecisionMaker(NewUpgradeSpec(nil))
	dir := filepath.Join(filepath.Dir(oldDir), "Avengers - Endgame (2019)")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(dir, "Avengers - Endgame (2019).mkv")
	if err := os.WriteFile(original, []byte("original movie"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc.existing = []*movies.MovieFile{
		{ID: "original-file", MovieID: svc.movie.ID, FilePath: original, Quality: "2160p BluRay"},
	}
	return p, svc, source, original
}

func TestNamedMovieReleaseKeepsOriginalAndEncore(t *testing.T) {
	for _, workflowLinked := range []bool{false, true} {
		name := "filename match"
		if workflowLinked {
			name = "workflow match"
		}
		t.Run(name, func(t *testing.T) {
			p, svc, _, original := encorePipeline(t)
			filename := strings.Replace(encoreFilename, "2160p", "720p", 1)
			source := filepath.Join(t.TempDir(), filename)
			if err := os.WriteFile(source, []byte("encore movie"), 0o600); err != nil {
				t.Fatal(err)
			}
			ev := &downloads.DownloadCompletedEvent{Title: filename}
			if workflowLinked {
				store, err := workflows.NewStore(p.db)
				if err != nil {
					t.Fatal(err)
				}
				ev.ClientID, ev.DownloadID = "client", "download"
				if err := store.Create(context.Background(), &workflows.Workflow{
					Type: workflows.TypeMovieSearch, State: workflows.StateImporting,
					MediaType: workflows.MediaTypeMovie, DownloadClientID: ev.ClientID, DownloadID: ev.DownloadID,
					Items: []workflows.Item{{MediaType: workflows.MediaTypeMovie, MediaID: svc.movie.ID}},
				}); err != nil {
					t.Fatal(err)
				}
				p.wfEngine = workflows.NewEngine(store, nil, p.logger)
			}
			scanned := p.scanSingleFile(context.Background(), source)
			if scanned.MatchedMediaID != svc.movie.ID || scanned.SuggestedAction != "import" {
				t.Fatalf("scan did not resolve original movie edition: %+v", scanned)
			}
			dest, err := p.importSingleFile(context.Background(), ev, source)
			if err != nil {
				t.Fatal(err)
			}
			if dest != filepath.Join(filepath.Dir(original), "Avengers - Endgame (2019) {edition-Encore}.mkv") {
				t.Fatalf("edition path = %q", dest)
			}
			for path, want := range map[string]string{original: "original movie", dest: "encore movie", source: "encore movie"} {
				got, err := os.ReadFile(path)
				if err != nil || string(got) != want {
					t.Fatalf("%s changed: %q, %v", path, got, err)
				}
			}
			if len(svc.files) != 1 || svc.files[0].MovieID != svc.movie.ID || svc.files[0].FilePath != dest {
				t.Fatalf("edition was not registered under original identity: %+v", svc.files)
			}
			roundTrip, err := p.matcher.Match(context.Background(), filepath.Base(dest))
			if err != nil || !roundTrip.Matched || roundTrip.Edition != "Encore" {
				t.Fatalf("edition filename failed round trip: %+v, %v", roundTrip, err)
			}
		})
	}
}

func TestNamedMovieReleaseRequiresProviderEvidence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		releases []metadata.MovieRelease
	}{
		{"missing release", nil},
		{"date without title", []metadata.MovieRelease{{ReleaseDate: "2026-09-25"}}},
		{"edition label without identity", []metadata.MovieRelease{{Title: "Encore", ReleaseDate: "2026-09-25"}}},
		{"wrong year", []metadata.MovieRelease{{Title: "Avengers Endgame: Encore", ReleaseDate: "2025-09-25"}}},
		{"different release", []metadata.MovieRelease{{Title: "Avengers Endgame: Another Cut", ReleaseDate: "2026-09-25"}}},
		{"invalid date", []metadata.MovieRelease{{Title: "Avengers Endgame: Encore", ReleaseDate: "2026-99-99"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, _, provider, _ := encorePipeline(t)
			provider.releases = tc.releases
			got, err := p.matcher.Match(context.Background(), encoreFilename)
			if err != nil || got.Matched {
				t.Fatalf("unverified release matched: %+v, %v", got, err)
			}
		})
	}
}

func TestNamedMovieReleaseLookupFailureIsRetryableAndVisible(t *testing.T) {
	p, _, provider, original := encorePipeline(t)
	provider.err = errors.New("provider temporarily unavailable")
	source := filepath.Join(t.TempDir(), encoreFilename)
	if err := os.WriteFile(source, []byte("encore movie"), 0o600); err != nil {
		t.Fatal(err)
	}
	scan := p.scanSingleFile(context.Background(), source)
	if scan.SuggestedAction != "manual_review" || !strings.Contains(scan.ImportError, provider.err.Error()) {
		t.Fatalf("scan hid provider error: %+v", scan)
	}
	_, err := p.importSingleFile(context.Background(), &downloads.DownloadCompletedEvent{Title: encoreFilename}, source)
	if !errors.Is(err, provider.err) || !isRetryableImportError(err) {
		t.Fatalf("lookup failure became terminal no-match: %v", err)
	}
	for path, want := range map[string]string{original: "original movie", source: "encore movie"} {
		content, err := os.ReadFile(path)
		if err != nil || string(content) != want {
			t.Fatalf("failed lookup changed %q: %q, %v", path, content, err)
		}
	}
}

func TestNamedMovieReleaseDoesNotAuthorizeConflictingYearOrTitle(t *testing.T) {
	p, _, _, _ := encorePipeline(t)
	match, err := p.matcher.Match(context.Background(), encoreFilename)
	if err != nil || !match.Matched {
		t.Fatalf("fixture release did not match: %+v, %v", match, err)
	}
	for _, title := range []string{
		"Avengers.Endgame.Encore.2025.1080p.mkv",
		"Avengers.Endgame.Other.Cut.2026.1080p.mkv",
		"Street.Fighter.2026.1080p.mkv",
	} {
		if err := verifyMovieImportYear(match, title, encoreFilename); err == nil {
			t.Fatalf("provider release authorized conflicting evidence %q", title)
		}
	}
}

func TestMovieQualityComparisonStaysWithinEdition(t *testing.T) {
	p, svc, _, original := encorePipeline(t)
	encore := filepath.Join(filepath.Dir(original), "Avengers - Endgame (2019) {edition-Encore}.mkv")
	svc.existing = append(svc.existing, &movies.MovieFile{
		MovieID: svc.movie.ID, FilePath: encore, Quality: "720p WEB-DL",
	})
	for _, tc := range []struct{ edition, want string }{
		{"", "2160p BluRay"}, {"Encore", "720p WEB-DL"}, {"Another edition", ""},
	} {
		candidate := &ImportCandidate{Match: &MatchResult{
			Matched: true, MediaType: MediaTypeMovie, MediaID: svc.movie.ID, Edition: tc.edition,
		}}
		if err := p.enrichCandidateQuality(context.Background(), candidate); err != nil {
			t.Fatal(err)
		}

		if candidate.ExistingQuality != tc.want {
			t.Fatalf("edition %q quality = %q, want %q", tc.edition, candidate.ExistingQuality, tc.want)
		}
	}
}

func TestEncoreDowngradeCannotReplaceExistingEdition(t *testing.T) {
	p, svc, _, original := encorePipeline(t)
	dest := filepath.Join(filepath.Dir(original), "Avengers - Endgame (2019) {edition-Encore}.mkv")
	if err := os.WriteFile(dest, []byte("existing encore"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc.existing = append(svc.existing, &movies.MovieFile{
		MovieID: svc.movie.ID, FilePath: dest, Quality: "2160p WEB-DL",
	})
	filename := strings.Replace(encoreFilename, "2160p", "720p", 1)
	source := filepath.Join(t.TempDir(), filename)
	if err := os.WriteFile(source, []byte("lower quality encore"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := p.importSingleFile(context.Background(), &downloads.DownloadCompletedEvent{Title: filename}, source)
	if err == nil || !strings.Contains(err.Error(), "lower than the existing quality") {
		t.Fatalf("edition downgrade was not rejected: %v", err)
	}
	for path, want := range map[string]string{
		source: "lower quality encore", dest: "existing encore", original: "original movie",
	} {
		content, err := os.ReadFile(path)
		if err != nil || string(content) != want {
			t.Fatalf("downgrade changed %q: %q, %v", path, content, err)
		}
	}
}
