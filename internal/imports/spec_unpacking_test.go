package imports

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
)

func TestBreakingDawnNumberedTitlePreviewsAndImports(t *testing.T) {
	ctx := context.Background()
	p, svc, _ := newCollisionPipeline(t, ImportModeCopy)
	profile := &qualityprofiles.QualityProfile{
		ID: "ultra-hd", Name: "Ultra-HD",
		Items: `[{"id":"bluray-2160p","name":"bluray-2160p","allowed":true}]`,
	}
	svc.movie = &movies.Movie{
		ID: "breaking-dawn-1", Title: "The Twilight Saga: Breaking Dawn - Part 1",
		Year: 2011, LibraryID: "lib", QualityProfileID: profile.ID,
	}
	p.matcher.moviesSvc = &literalSearchMoviesService{
		collisionMoviesService: svc, candidates: []*movies.Movie{svc.movie},
	}
	profiles := &stubProfileStore{profile: profile}
	p.decisions = NewDecisionMaker(
		&FreeSpaceSpec{}, &UnpackingSpec{}, &DangerousFileSpec{},
		NewUpgradeSpec(profiles), NewCustomFormatSpec(profiles, nil),
	)
	title := "The.Twilight.Saga.Breaking.Dawn.-.Part.1.2011.2160p.4K.BluRay.x265.10bit.AAC5.1-[YTS.MX].mkv"
	root := t.TempDir()
	source := filepath.Join(root, title)
	if err := os.WriteFile(source, []byte("complete movie"), 0o600); err != nil {
		t.Fatal(err)
	}
	if preview := p.previewSingleFile(ctx, source); preview.Action != "import" || preview.MediaID != svc.movie.ID {
		t.Fatalf("numbered movie title rejected by preview: %+v", preview)
	}
	paths, err := p.processImport(ctx, &downloads.DownloadCompletedEvent{Title: title}, root)
	if err != nil || len(paths) != 1 {
		t.Fatalf("numbered movie title rejected by import: %v, %v", paths, err)
	}
	if len(svc.files) != 1 || svc.files[0].Quality != "bluray-2160p" {
		t.Fatalf("wrong movie registration: %+v", svc.files)
	}
	for _, path := range []string{source, paths[0]} {
		content, err := os.ReadFile(path)
		if err != nil || string(content) != "complete movie" {
			t.Fatalf("import altered movie contents at %q: %q, %v", path, content, err)
		}
	}
}

func TestUnpackingSpecDistinguishesNumberedTitlesFromPartialFiles(t *testing.T) {
	for _, tc := range []struct {
		path    string
		blocked bool
	}{
		{"/downloads/The.Twilight.Saga.Breaking.Dawn.-.Part.1.2011.2160p.4K.BluRay.x265.10bit.AAC5.1-[YTS.MX].mkv", false},
		{"/downloads/The.Twilight.Saga.Breaking.Dawn.-.Part.2.2012.2160p.4K.BluRay.x265.10bit.AAC5.1-[YTS.MX].mkv", false},
		{"/downloads/Harry.Potter.And.The.Deathly.Hallows.Part.2.2011.1080p.mkv", false},
		{"/downloads/Film.PART.01.mkv", false},
		{"/downloads/Film.Part.1.Part.2.mkv", false},
		{"/downloads/Film.mkv.part", true},
		{"/downloads/Film.part.mkv", true},
		{"/downloads/Show.part.S01E02.mkv", true},
		{"/downloads/Film.mkv.part.1", true},
		{"/downloads/Film.Part.1.mkv.part", true},
		{"/downloads/Film.Part.1.part.mkv", true},
		{"/downloads/Film.Part.1.mkv.!qB", true},
		{"/downloads/Film.Part.1.mkv.downloading", true},
		{"/downloads/Film.Part.1._unpack.mkv", true},
		{"/downloads/_UNPACK_release/Film.Part.1.mkv", true},
		{"/downloads/_UNRARRED_release/Film.Part.1.mkv", true},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rejection := (&UnpackingSpec{}).IsSatisfiedBy(context.Background(), &ImportCandidate{SourcePath: tc.path})
			if (rejection != nil) != tc.blocked {
				t.Fatalf("rejection=%+v, want blocked=%t", rejection, tc.blocked)
			}
			if rejection != nil && rejection.Reason != RejectionUnpacking {
				t.Fatalf("wrong rejection reason: %+v", rejection)
			}
		})
	}
}
