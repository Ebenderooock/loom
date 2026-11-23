package customformats

import (
	"encoding/json"
	"testing"

	"github.com/ebenderooock/loom/internal/qualityprofiles"
)

func testProfile(t *testing.T, items ...qualityprofiles.QualityItem) *qualityprofiles.QualityProfile {
	t.Helper()
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	return &qualityprofiles.QualityProfile{Items: string(raw)}
}

func testTier(id, name string) Tier {
	return Tier{ID: id, Name: name, Specifications: []Specification{
		{Implementation: ImplResolution, Fields: map[string]any{"value": "1080p"}},
		{Implementation: ImplCodec, Fields: map[string]any{"value": "x265"}},
		{Implementation: ImplSource, Fields: map[string]any{"value": "unknown"}},
	}}
}

func TestResolveUnspecifiedSource(t *testing.T) {
	tier := testTier("tier-1", "1080p HEVC — unspecified source")
	profile := testProfile(t, qualityprofiles.QualityItem{ID: tier.ID, Name: tier.Name, Kind: CustomKind, Allowed: true})
	for _, tc := range []struct {
		title string
		match bool
	}{
		{"The.Body.2026.S01E08.1080p.HEVC.x265-MeGusta[EZTVx.to].mkv", true},
		{"The.Body.2026.S01E08.1080p.x264-MeGusta.mkv", false},
		{"The.Body.2026.S01E08.720p.x265-MeGusta.mkv", false},
		{"The.Body.2026.S01E08.1080p.WEB-DL.x265-MeGusta.mkv", false},
	} {
		t.Run(tc.title, func(t *testing.T) {
			ri := ParseReleaseName(tc.title)
			got, err := ResolveTier(profile, []Tier{tier}, ri, "")
			if err != nil || (got != nil) != tc.match {
				t.Fatalf("got %+v, %v; want match %t", got, err, tc.match)
			}
			if tc.match && ri.Source != "" {
				t.Fatalf("source was changed: %q", ri.Source)
			}
		})
	}
}

func TestOrderExclusionAndRename(t *testing.T) {
	one, two := testTier("one", "Old"), testTier("two", "Second")
	// Match any source here to verify that an explicitly excluded built-in
	// source cannot be bypassed by a broad matcher.
	one.Specifications = one.Specifications[:2]
	two.Specifications = two.Specifications[:2]
	release := ParseReleaseName("Film.2026.1080p.WEB-DL.x265-GROUP")
	customOne := qualityprofiles.QualityItem{ID: "one", Name: "Old", Kind: CustomKind, Allowed: true}
	customTwo := qualityprofiles.QualityItem{ID: "two", Name: "Second", Kind: CustomKind, Allowed: true}
	base := qualityprofiles.QualityItem{ID: "web", Name: "webdl-1080p", Allowed: true}
	profile := testProfile(t, customOne, customTwo, base)
	one.Name = "Renamed"
	got, err := ResolveTier(profile, []Tier{one, two}, release, "web")
	if err != nil || got == nil || got.ID != "one" || got.Name != "Renamed" || got.Rank != 0 {
		t.Fatalf("unexpected resolution %+v, %v", got, err)
	}
	profile = testProfile(t, customTwo, customOne, base)
	got, err = ResolveTier(profile, []Tier{one, two}, release, "web")
	if err != nil || got == nil || got.ID != "two" {
		t.Fatalf("reorder not honored: %+v, %v", got, err)
	}
	base.Allowed = false
	profile = testProfile(t, customOne, base)
	got, err = ResolveTier(profile, []Tier{one}, release, "web")
	if err != nil || got != nil {
		t.Fatalf("excluded source bypassed: %+v, %v", got, err)
	}
}

func TestResolveRejectsInvalidOrDeletedTiers(t *testing.T) {
	profile := testProfile(t, qualityprofiles.QualityItem{ID: "one", Kind: CustomKind, Allowed: true})
	release := ParseReleaseName("Film.1080p.x265")
	if _, err := ResolveTier(profile, nil, release, ""); err == nil {
		t.Fatal("expected missing tier error")
	}

	tier := testTier("one", "Tier")
	tier.Specifications = append(tier.Specifications, Specification{Implementation: ImplFileInfo})
	if _, err := ResolveTier(profile, []Tier{tier}, release, ""); err == nil {
		t.Fatal("expected unsupported matcher error")
	}
}

func TestEditedMatcherTakesEffect(t *testing.T) {
	tier := testTier("one", "Tier")
	profile := testProfile(t, qualityprofiles.QualityItem{ID: tier.ID, Kind: CustomKind, Allowed: true})
	release := ParseReleaseName("Film.1080p.x265")
	got, err := ResolveTier(profile, []Tier{tier}, release, "")
	if err != nil || got == nil {
		t.Fatalf("expected match: %+v, %v", got, err)
	}

	tier.Specifications[1].Fields["value"] = "x264"
	got, err = ResolveTier(profile, []Tier{tier}, release, "")
	if err != nil || got != nil {
		t.Fatalf("edited rule not honored: %+v, %v", got, err)
	}
}

func TestUnknownTierAndCustomTierOrder(t *testing.T) {
	tier := testTier("one", "HEVC")
	custom := qualityprofiles.QualityItem{ID: "one", Kind: CustomKind, Allowed: true}
	unknown := qualityprofiles.QualityItem{ID: "unknown", Name: "unknown", Allowed: true}
	release := ParseReleaseName("Film.1080p.x265")
	profile := testProfile(t, custom, unknown)
	got, err := ResolveTier(profile, []Tier{tier}, release, BaseTierID(profile, release.Quality))
	if err != nil || got == nil || got.ID != "one" {
		t.Fatalf("custom tier should win: %+v %v", got, err)
	}
	profile = testProfile(t, unknown, custom)
	got, err = ResolveTier(profile, []Tier{tier}, release, BaseTierID(profile, release.Quality))
	if err != nil || got == nil || got.ID != "unknown" {
		t.Fatalf("unknown tier should win: %+v %v", got, err)
	}
	unknown.Allowed = false
	profile = testProfile(t, unknown, custom)
	got, err = ResolveTier(profile, []Tier{tier}, release, BaseTierID(profile, release.Quality))
	if err != nil || got == nil || got.ID != "one" {
		t.Fatalf("disabled Unknown must not block custom tier: %+v %v", got, err)
	}
}
