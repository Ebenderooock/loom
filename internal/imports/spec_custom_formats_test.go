package imports

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/customformats"
	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/parser"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
)

type stubCustomFormats struct {
	formats []customformats.CustomFormat
	err     error
}

func (s *stubCustomFormats) List(context.Context) ([]customformats.CustomFormat, error) {
	return s.formats, s.err
}

func bodyFormat() customformats.CustomFormat {
	return customformats.CustomFormat{ID: "scene-web", Name: "Scene WEB", Specifications: []customformats.Specification{
		{Implementation: customformats.ImplSource, Fields: map[string]any{"value": "WEB-DL"}},
		{Implementation: customformats.ImplCodec, Fields: map[string]any{"value": "x264"}},
		{Implementation: customformats.ImplQuality, Fields: map[string]any{"value": "webdl-1080p"}},
	}}
}

func TestCustomFormatSpecUsesProfilePointsAndCurrentRules(t *testing.T) {
	profile := unrestrictedProfile(t)
	profile.MinFormatScore = 100
	profile.FormatItems = []qualityprofiles.FormatItem{{FormatID: "scene-web", Score: 100}}
	formats := &stubCustomFormats{formats: []customformats.CustomFormat{bodyFormat()}}
	spec := NewCustomFormatSpec(&stubProfileStore{profile: profile}, formats)
	file := "the.body.2026.s01e07.1080p.web.h264-cakes[EZTVx.to].mkv"
	candidate := &ImportCandidate{SourcePath: file, IncomingRelease: parser.Parse(file), QualityProfileID: profile.ID}
	// Score problems never block an import; they are reported as warnings.
	warning := func() string {
		t.Helper()
		candidate.QualityWarnings = nil
		if rejection := spec.IsSatisfiedBy(context.Background(), candidate); rejection != nil {
			t.Fatalf("custom format score must not block: %+v", rejection)
		}
		return strings.Join(candidate.QualityWarnings, "; ")
	}
	if w := warning(); w != "" {
		t.Fatalf("matching WEB-DL/x264/quality rules should score 100: %s", w)
	}
	profile.FormatItems[0].Score = -10
	if w := warning(); !strings.Contains(w, "score -10") {
		t.Fatalf("negative profile points must be reported: %s", w)
	}
	profile.FormatItems[0].Score = 100
	formats.formats[0].Specifications[0].Fields["value"] = "BluRay"
	if w := warning(); w == "" {
		t.Fatal("edited custom format must apply without restarting the importer")
	}
	formats.err = errors.New("format storage unavailable")
	if w := warning(); !strings.Contains(w, formats.err.Error()) {
		t.Fatalf("format load errors must be explicit: %s", w)
	}
	candidate.QualityProfileID = ""
	if w := warning(); w != "" {
		t.Fatalf("unassigned profiles must not apply other profiles' rules: %s", w)
	}
}

func TestBodyHEVCImportWarnsOnUnknownTierAndFormatMinimum(t *testing.T) {
	for _, tc := range []struct {
		name    string
		unknown bool
		minimum int
		warning string
	}{
		{"live profile permits first import", true, 1, ""},
		{"explicit unknown exclusion", false, 1, "not allowed by profile"},
		{"format minimum is reported", true, 3, "score 2 is below minimum 3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			profile := unrestrictedProfile(t)
			if tc.unknown {
				profile.Items = `[{"id":"unknown","name":"unknown","allowed":true}]`
			} else {
				profile.Items = `[{"id":"unknown","name":"unknown","allowed":false}]`
			}
			profile.MinFormatScore = tc.minimum
			profile.FormatItems = []qualityprofiles.FormatItem{
				{FormatID: "1080p", Score: 1}, {FormatID: "1080p-hvec", Score: 1},
			}
			fixture := newPackPipeline(t, ImportModeCopy, func(opts *PipelineOptions) {
				opts.JobWorkers = -1
				opts.QualityProfiles = &stubProfileStore{profile: profile}
				if _, err := opts.DB.Exec(`CREATE TABLE custom_formats (
					id TEXT PRIMARY KEY, name TEXT, include_when_renaming INTEGER,
					specifications TEXT, created_at TEXT, updated_at TEXT)`); err != nil {
					t.Fatal(err)
				}
				for _, format := range []customformats.CustomFormat{
					{ID: "1080p", Name: "1080p", Specifications: []customformats.Specification{
						{Implementation: customformats.ImplReleaseTitle, Required: true, Fields: map[string]any{"value": "1080p.HEVC"}},
					}},
					{ID: "1080p-hvec", Name: "x265", Specifications: []customformats.Specification{
						{Implementation: customformats.ImplCodec, Fields: map[string]any{"value": "x265"}},
						{Implementation: customformats.ImplResolution, Fields: map[string]any{"value": "1080p"}},
					}},
				} {
					if err := customformats.NewStore(opts.DB).Create(ctx, &format); err != nil {
						t.Fatal(err)
					}
				}
			})
			fixture.svc.show.Title, fixture.svc.show.QualityProfileID = "The Body", profile.ID
			fixture.svc.eps[0].SeasonID, fixture.svc.eps[0].EpisodeNumber = "season-1", 8
			pipeline, err := NewPipeline(fixture.opts)
			if err != nil {
				t.Fatal(err)
			}
			defer pipeline.Stop()
			root := t.TempDir()
			title := "The.Body.2026.S01E08.1080p.HEVC.x265-MeGusta[EZTVx.to].mkv"
			source := filepath.Join(root, title)
			if err := os.WriteFile(source, []byte("complete episode"), 0o600); err != nil {
				t.Fatal(err)
			}
			preview := pipeline.previewSingleFile(ctx, source)
			warnings := strings.Join(preview.Warnings, "; ")
			if preview.Action != "import" || (tc.warning == "") != (warnings == "") || !strings.Contains(warnings, tc.warning) {
				t.Fatalf("unexpected preview: %+v", preview)
			}
			_, err = pipeline.processImport(ctx, &downloads.DownloadCompletedEvent{DownloadID: "body-hevc", Title: title}, root)
			if err != nil {
				t.Fatalf("quality must not block the import: %v", err)
			}
			if len(fixture.svc.files) != 1 || fixture.svc.files[0].Quality != "unknown-1080p" {
				t.Fatalf("import must retain the actual source-unknown metadata: %+v", fixture.svc.files)
			}
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("source must remain available: %v", err)
			}
		})
	}
}

func TestBodyImportWarnsOnCustomProfileWithoutChangingQuality(t *testing.T) {
	for _, tc := range []struct {
		name       string
		score      int
		disallowed bool
		warned     bool
	}{
		{"matching score", 100, false, false},
		{"below minimum", 99, false, true},
		{"negative penalty", -100, false, true},
		{"score cannot hide excluded quality", 10000, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			profile := anyProfile()
			profile.MinFormatScore = 100
			profile.FormatItems = []qualityprofiles.FormatItem{{FormatID: "scene-web", Score: tc.score}}
			if tc.disallowed {
				profile.Items = `[{"id":"bluray-1080p","name":"bluray-1080p","allowed":true}]`
			}
			fixture := newPackPipeline(t, ImportModeCopy, func(opts *PipelineOptions) {
				opts.JobWorkers = -1
				opts.QualityProfiles = &stubProfileStore{profile: profile}
				if _, err := opts.DB.Exec(`CREATE TABLE custom_formats (
					id TEXT PRIMARY KEY, name TEXT, include_when_renaming INTEGER,
					specifications TEXT, created_at TEXT, updated_at TEXT)`); err != nil {
					t.Fatal(err)
				}
				format := bodyFormat()
				if err := customformats.NewStore(opts.DB).Create(ctx, &format); err != nil {
					t.Fatal(err)
				}
			})
			fixture.svc.show.Title, fixture.svc.show.QualityProfileID = "The Body", profile.ID
			fixture.svc.eps[0].SeasonID, fixture.svc.eps[0].EpisodeNumber = "season-1", 7
			pipeline, err := NewPipeline(fixture.opts)
			if err != nil {
				t.Fatal(err)
			}
			defer pipeline.Stop()
			root := t.TempDir()
			source := filepath.Join(root, "the.body.2026.s01e07.1080p.web.h264-cakes[EZTVx.to].mkv")
			if err := os.WriteFile(source, []byte("complete episode"), 0o600); err != nil {
				t.Fatal(err)
			}
			preview := pipeline.previewSingleFile(ctx, source)
			if preview.Action != "import" || (len(preview.Warnings) != 0) != tc.warned {
				t.Fatalf("preview disagrees with profile: %+v", preview)
			}
			paths, err := pipeline.processImport(ctx, &downloads.DownloadCompletedEvent{
				DownloadID: "body", Title: "The Body 2026 S01E07 1080p WEB H264-CAKES EZTV",
			}, root)
			if err != nil {
				t.Fatalf("profile import: paths=%v err=%v", paths, err)
			}
			if len(fixture.svc.files) != 1 || fixture.svc.files[0].Quality != "webdl-1080p" {
				t.Fatalf("import must retain detected base quality: %+v", fixture.svc.files)
			}
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("source must remain available: %v", err)
			}
		})
	}
}

func TestCustomFormatSpecWarnsInsteadOfRejecting(t *testing.T) {
	profile := unrestrictedProfile(t)
	profile.MinFormatScore = 100
	profile.FormatItems = []qualityprofiles.FormatItem{{FormatID: "scene-web", Score: -10}}
	spec := NewCustomFormatSpec(&stubProfileStore{profile: profile}, &stubCustomFormats{formats: []customformats.CustomFormat{bodyFormat()}})
	file := "the.body.2026.s01e07.1080p.web.h264-cakes[EZTVx.to].mkv"
	candidate := &ImportCandidate{SourcePath: file, IncomingRelease: parser.Parse(file), QualityProfileID: profile.ID}
	if rejection := spec.IsSatisfiedBy(context.Background(), candidate); rejection != nil || len(candidate.QualityWarnings) != 1 {
		t.Fatalf("low score: rejection=%+v warnings=%v", rejection, candidate.QualityWarnings)
	}
}
