package imports

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/customformats"
	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
)

func TestCustomTierEpisodeImportAndPreview(t *testing.T) {
	ctx := context.Background()
	profile := &qualityprofiles.QualityProfile{ID: "profile", Name: "Custom", UpgradeAllowed: true,
		Items: `[{"id":"ct_test","name":"HEVC","kind":"custom","allowed":true},{"id":"web","name":"webdl-1080p","allowed":false}]`,
		FormatItems: []qualityprofiles.FormatItem{
			{FormatID: "1080p", Score: 1}, {FormatID: "1080p-hevc", Score: 1},
		}}
	f := newPackPipeline(t, ImportModeCopy, func(opts *PipelineOptions) {
		opts.JobWorkers = -1
		opts.QualityProfiles = &stubProfileStore{profile: profile}
		_, err := opts.DB.Exec(`CREATE TABLE custom_quality_tiers (id TEXT PRIMARY KEY, name TEXT, specifications TEXT, created_at TEXT, updated_at TEXT);
			INSERT INTO custom_quality_tiers VALUES ('ct_test','HEVC',
			'[{"implementation":"ResolutionSpec","fields":{"value":"1080p"}},{"implementation":"CodecSpec","fields":{"value":"x265"}},{"implementation":"SourceSpec","fields":{"value":"unknown"}}]', '', '')`)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := opts.DB.Exec(`CREATE TABLE custom_formats (
			id TEXT PRIMARY KEY, name TEXT, include_when_renaming INTEGER,
			specifications TEXT, created_at TEXT, updated_at TEXT)`); err != nil {
			t.Fatal(err)
		}
		for _, format := range []customformats.CustomFormat{
			{ID: "1080p", Name: "1080p", Specifications: []customformats.Specification{
				{Implementation: customformats.ImplReleaseTitle, Required: true, Fields: map[string]any{"value": "1080p.HEVC"}},
			}},
			{ID: "1080p-hevc", Name: "x265", Specifications: []customformats.Specification{
				{Implementation: customformats.ImplCodec, Fields: map[string]any{"value": "x265"}},
				{Implementation: customformats.ImplResolution, Fields: map[string]any{"value": "1080p"}},
			}},
		} {
			if err := customformats.NewStore(opts.DB).Create(ctx, &format); err != nil {
				t.Fatal(err)
			}
		}
	})
	f.svc.show.Title, f.svc.show.QualityProfileID = "The Body", profile.ID
	f.svc.eps[0].SeasonID, f.svc.eps[0].EpisodeNumber = "season-1", 8
	pipeline, err := NewPipeline(f.opts)
	if err != nil {
		t.Fatal(err)
	}
	defer pipeline.Stop()
	root := t.TempDir()
	title := "The.Body.2026.S01E08.1080p.HEVC.x265-MeGusta[EZTVx.to].mkv"
	path := filepath.Join(root, title)
	if err := os.WriteFile(path, []byte("complete episode"), 0o600); err != nil {
		t.Fatal(err)
	}
	profile.MinFormatScore = 3
	if preview := pipeline.previewSingleFile(ctx, path); preview.Action != "skip" ||
		!strings.Contains(preview.Reason, "score 2 is below minimum 3") {
		t.Fatalf("format minimum must reject even a matching tier: %+v", preview)
	}
	event := &downloads.DownloadCompletedEvent{DownloadID: "body", Title: title}
	if _, err := pipeline.processImport(ctx, event, root); err == nil ||
		!strings.Contains(err.Error(), "score 2 is below minimum 3") || len(f.svc.files) != 0 {
		t.Fatalf("real import bypassed the score minimum: %v", err)
	}
	profile.MinFormatScore = 1
	if preview := pipeline.previewSingleFile(ctx, path); preview.Action != "import" {
		t.Fatalf("preview rejected: %+v", preview)
	}
	_, err = pipeline.processImport(ctx, event, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.svc.files) != 1 || f.svc.files[0].Quality != customformats.StoredTierID("ct_test") {
		t.Fatalf("tier identity not stored: %+v", f.svc.files)
	}
	if f.svc.files[0].Source != "" {
		t.Fatalf("import fabricated a source: %q", f.svc.files[0].Source)
	}
}

type tierStub []customformats.Tier

func (t tierStub) ListTiers(context.Context) ([]customformats.Tier, error) { return t, nil }

func TestCustomTierUpgradeProtection(t *testing.T) {
	profile := &qualityprofiles.QualityProfile{ID: "p", Name: "Custom", UpgradeAllowed: true, Cutoff: "ct_best",
		Items: `[{"id":"ct_best","name":"Best","kind":"custom","allowed":true},{"id":"ct_lower","name":"Lower","kind":"custom","allowed":true}]`}
	tiers := tierStub{
		{ID: "ct_best", Name: "Renamed", Specifications: []customformats.Specification{{Implementation: customformats.ImplCodec, Fields: map[string]any{"value": "x265"}}}},
		{ID: "ct_lower", Name: "Lower", Specifications: []customformats.Specification{{Implementation: customformats.ImplCodec, Fields: map[string]any{"value": "x264"}}}},
	}
	spec := NewUpgradeSpec(&stubProfileStore{profile: profile}, tiers)
	candidate := &ImportCandidate{SourcePath: "Film.1080p.x264.mkv", QualityProfileID: "p", ExistingQuality: customformats.StoredTierID("ct_best")}
	if rejection := spec.IsSatisfiedBy(context.Background(), candidate); rejection == nil {
		t.Fatal("lower custom tier replaced best")
	}
	candidate.SourcePath = "Film.1080p.x265.mkv"
	candidate.ExistingQuality = customformats.StoredTierID("ct_lower")
	if rejection := spec.IsSatisfiedBy(context.Background(), candidate); rejection != nil || candidate.ResolvedTier != customformats.StoredTierID("ct_best") {
		t.Fatalf("renamed tier did not upgrade: %+v", rejection)
	}
	profile.Cutoff = "ct_lower"
	if rejection := spec.IsSatisfiedBy(context.Background(), candidate); rejection == nil {
		t.Fatal("cutoff did not stop upgrade")
	}
}

func TestCustomTierCannotBypassExcludedKnownSource(t *testing.T) {
	profile := &qualityprofiles.QualityProfile{ID: "p", Name: "Custom", UpgradeAllowed: true,
		Items: `[{"id":"ct_test","name":"HEVC","kind":"custom","allowed":true},{"id":"web","name":"webdl-1080p","allowed":false}]`}
	tiers := tierStub{{ID: "ct_test", Name: "HEVC", Specifications: []customformats.Specification{
		{Implementation: customformats.ImplCodec, Fields: map[string]any{"value": "x265"}},
	}}}
	candidate := &ImportCandidate{SourcePath: "Film.1080p.WEB-DL.x265.mkv", QualityProfileID: "p"}
	if rejection := NewUpgradeSpec(&stubProfileStore{profile: profile}, tiers).IsSatisfiedBy(context.Background(), candidate); rejection == nil {
		t.Fatal("custom matcher bypassed disabled WEB quality")
	}
}
