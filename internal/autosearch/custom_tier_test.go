package autosearch

import (
	"testing"

	"github.com/ebenderooock/loom/internal/customformats"
	"github.com/ebenderooock/loom/internal/indexers"
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
