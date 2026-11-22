package autosearch

import (
	"testing"

	"github.com/ebenderooock/loom/internal/customformats"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
)

func TestStoredCustomTierUsesCurrentProfileOrder(t *testing.T) {
	stored := customformats.StoredTierID("ct_test")
	for _, rank := range []int{0, 3} {
		got, ok := matchStoredQualityToTier(stored, nil, map[string]int{"ct_test": rank})
		if !ok || got != rank {
			t.Fatalf("stored identity at rank %d: %d %v", rank, got, ok)
		}
	}
	if _, ok := matchStoredQualityToTier(stored, nil, map[string]int{}); ok {
		t.Fatal("removed tier must not be assigned a guessed rank")
	}
}

func TestCustomTierSearchPolicy(t *testing.T) {
	engine, defs := explanationEngine()
	profile := &qualityprofiles.QualityProfile{UpgradeAllowed: true,
		Items: `[{"id":"ct_test","name":"HEVC","kind":"custom","allowed":true},{"id":"web","name":"webdl-1080p","allowed":false}]`}
	tiers := []customformats.Tier{{ID: "ct_test", Name: "HEVC", Specifications: []customformats.Specification{
		{Implementation: customformats.ImplResolution, Fields: map[string]any{"value": "1080p"}},
		{Implementation: customformats.ImplCodec, Fields: map[string]any{"value": "x265"}},
	}}}
	req := SearchRequest{Title: "The Body", Season: 1, Episode: 8}
	check := func(title string, existing existingQuality) ScoredRelease {
		return engine.evaluateResult(req, indexers.Result{Title: title}, defs,
			map[string]int{"ct_test": 0}, map[string]bool{"ct_test": true}, nil,
			profile, existing, -1, false, tiers)
	}

	title := "The.Body.2026.S01E08.1080p.HEVC.x265-MeGusta[EZTVx.to].mkv"
	sr := check(title, existingQuality{})
	if sr.Rejected || sr.QualityDef.ID != "ct_test" || sr.Parsed.Source != "" {
		t.Fatalf("expected custom source-unknown tier: %+v", sr)
	}
	sr = check("The.Body.2026.S01E08.1080p.WEB-DL.x265-GROUP", existingQuality{})
	if !sr.Rejected || sr.RejectReason != "quality_not_allowed" {
		t.Fatalf("excluded WEB source bypassed: %+v", sr)
	}
	profile.MinFormatScore = 1
	sr = check(title, existingQuality{})
	if !sr.Rejected || sr.RejectReason != "below_min_format_score" {
		t.Fatalf("minimum score bypassed: %+v", sr)
	}
	profile.MinFormatScore = 0
	sr = check(title, existingQuality{HasFile: true, HasKnownTier: true, Tier: 0})
	if !sr.Rejected || sr.RejectReason != "not_an_upgrade" {
		t.Fatalf("same-tier ordinary release replaced existing: %+v", sr)
	}
}

func TestSourcelessReleaseIsAssumedWebDLNotHDTV(t *testing.T) {
	// Seed order lists HDTV before WEB-DL; the old fallback picked HDTV.
	defs := []*movies.QualityDefinition{
		{ID: "hdtv-1080p", Name: "hdtv-1080p", Title: "HDTV 1080p", Source: "TV", Resolution: "1080p"},
		{ID: "webdl-1080p", Name: "webdl-1080p", Title: "WEB-DL 1080p", Source: "Web", Resolution: "1080p"},
		{ID: "bluray-1080p", Name: "bluray-1080p", Title: "Bluray 1080p", Source: "BluRay", Resolution: "1080p"},
	}
	profile := &qualityprofiles.QualityProfile{Name: "HD-1080p", UpgradeAllowed: true,
		Items: `[{"id":"bluray-1080p","name":"bluray-1080p","allowed":true},{"id":"webdl-1080p","name":"webdl-1080p","allowed":true},{"id":"hdtv-1080p","name":"hdtv-1080p","allowed":true}]`}
	allowed := map[string]int{"bluray-1080p": 0, "webdl-1080p": 1, "hdtv-1080p": 2}
	enabled := map[string]bool{"bluray-1080p": true, "webdl-1080p": true, "hdtv-1080p": true}
	engine := &Engine{cfEngine: customformats.NewEngine(nil)}
	req := SearchRequest{Title: "Chicago PD", Season: 14, Episode: 6}

	sr := engine.evaluateResult(req, indexers.Result{Title: "Chicago PD S14E06 1080p HEVC x265-MeGusta EZTV"},
		defs, allowed, enabled, nil, profile, existingQuality{}, -1, false)
	if sr.Rejected || sr.QualityDef.ID != "webdl-1080p" || !sr.QualityAssumed ||
		sr.QualityStored != "webdl-1080p" || sr.QualityDetected != "unknown-1080p" {
		t.Fatalf("source-less release: def=%v assumed=%t stored=%q detected=%q rejected=%s",
			sr.QualityDef, sr.QualityAssumed, sr.QualityStored, sr.QualityDetected, sr.RejectReason)
	}
	sr = engine.evaluateResult(req, indexers.Result{Title: "Chicago PD S14E06 1080p HDTV x264-GROUP"},
		defs, allowed, enabled, nil, profile, existingQuality{}, -1, false)
	if sr.Rejected || sr.QualityDef.ID != "hdtv-1080p" || sr.QualityAssumed {
		t.Fatalf("stated source must not be assumed: %+v", sr)
	}
}
