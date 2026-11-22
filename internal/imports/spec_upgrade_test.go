package imports

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/parser"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
)

func unrestrictedProfile(t *testing.T) *qualityprofiles.QualityProfile {
	t.Helper()
	names := []string{
		"bluray-2160p-remux", "bluray-2160p", "webrip-2160p", "webdl-2160p", "hdtv-2160p",
		"bluray-1080p-remux", "bluray-1080p", "webrip-1080p", "webdl-1080p", "hdtv-1080p",
		"bluray-720p", "webrip-720p", "webdl-720p", "hdtv-720p", "dvd", "webdl-480p", "sdtv",
	}
	items := make([]qualityprofiles.QualityItem, 0, len(names))
	for _, name := range names {
		items = append(items, qualityprofiles.QualityItem{ID: name, Name: name, Allowed: true})
	}
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	return &qualityprofiles.QualityProfile{ID: "prof-1", Name: "Any", UpgradeAllowed: true, Items: string(raw)}
}

func TestUnknownQualityWarnsButCannotReplaceKnownQuality(t *testing.T) {
	for _, tc := range []struct {
		name      string
		profile   *qualityprofiles.QualityProfile
		existing  string
		warned    bool
		downgrade bool
	}{
		{"unrestricted first import", unrestrictedProfile(t), "", false, false},
		{"restricted first import", anyProfile(), "", true, false},
		{"unknown replacement", unrestrictedProfile(t), "bluray-1080p", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &ImportCandidate{SourcePath: "Episode 1 Title.mkv", QualityProfileID: tc.profile.ID, ExistingQuality: tc.existing}
			rejection := NewUpgradeSpec(&stubProfileStore{profile: tc.profile}).IsSatisfiedBy(context.Background(), c)
			if (rejection != nil && rejection.Reason == RejectionDowngrade) != tc.downgrade || (rejection == nil) == tc.downgrade ||
				(len(c.QualityWarnings) != 0) != tc.warned {
				t.Fatalf("rejection=%+v warnings=%v; want downgrade=%v warned=%v", rejection, c.QualityWarnings, tc.downgrade, tc.warned)
			}
		})
	}
	profile := unrestrictedProfile(t)
	items, err := profile.ParseItems()
	if err != nil {
		t.Fatal(err)
	}
	items[0].Allowed = false
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	profile.Items = string(raw)
	c := &ImportCandidate{SourcePath: "Episode 1 Title.mkv", QualityProfileID: profile.ID}
	if r := NewUpgradeSpec(&stubProfileStore{profile: profile}).IsSatisfiedBy(context.Background(), c); r != nil || len(c.QualityWarnings) == 0 {
		t.Fatalf("profile named Any with an explicit exclusion must warn about unknown quality: rejection=%+v warnings=%v", r, c.QualityWarnings)
	}
}

func TestSourceUnknownQualityUsesAllowedUnknownTierForFirstImport(t *testing.T) {
	explicitUnknown := &qualityprofiles.QualityProfile{
		ID: "prof-1", Name: "Custom", UpgradeAllowed: true,
		Items: `[{"id":"unknown","name":"unknown","allowed":true},{"id":"webdl-1080p","name":"webdl-1080p","allowed":false}]`,
	}
	excludedUnknown := *explicitUnknown
	excludedUnknown.Items = `[{"id":"unknown","name":"unknown","allowed":false},{"id":"webdl-1080p","name":"webdl-1080p","allowed":true}]`
	for _, tc := range []struct {
		name     string
		profile  *qualityprofiles.QualityProfile
		title    string
		existing string
		allowed  bool
	}{
		{"allowed unknown tier", explicitUnknown, "The.Body.2026.S01E08.1080p.HEVC.x265-MeGusta[EZTVx.to].mkv", "", true},
		{"unrestricted legacy profile", unrestrictedProfile(t), "The.Body.2026.S01E08.1080p.HEVC.x265-MeGusta[EZTVx.to].mkv", "", true},
		// Like search, a source-less 1080p release is assumed WEB-DL 1080p.
		{"unknown tier excluded assumes WEB-DL", &excludedUnknown, "The.Body.2026.S01E08.1080p.HEVC.x265-MeGusta[EZTVx.to].mkv", "", true},
		{"restricted profile assumes WEB-DL", anyProfile(), "The.Body.2026.S01E08.1080p.HEVC.x265-MeGusta[EZTVx.to].mkv", "", true},
		{"explicit Unknown tier accepts when the assumed tier is disabled", explicitUnknownOnly(), "Show.S01E01.720p.x265-GROUP.mkv", "", true},
		// Profile problems warn rather than block.
		{"excluded known source imports with a warning", explicitUnknown, "The.Body.2026.S01E08.1080p.WEB.h264-GROUP.mkv", "", true},
		// This profile ranks Unknown above WEB-DL, so the replacement is an upgrade.
		{"unknown ranked above existing replaces it", explicitUnknown, "The.Body.2026.S01E08.1080p.HEVC.x265-MeGusta[EZTVx.to].mkv", "webdl-1080p", true},
		{"resolution downgrade stays rejected", unrestrictedProfile(t), "The.Body.2026.S01E08.1080p.HEVC.x265-MeGusta[EZTVx.to].mkv", "webdl-2160p", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, withRelease := range []bool{false, true} {
				candidate := &ImportCandidate{SourcePath: tc.title, QualityProfileID: tc.profile.ID, ExistingQuality: tc.existing}
				if withRelease {
					candidate.IncomingRelease = parser.Parse(tc.title)
					candidate.SourcePath = "renamed.mkv"
				}
				rejection := NewUpgradeSpec(&stubProfileStore{profile: tc.profile}).IsSatisfiedBy(context.Background(), candidate)
				if (rejection == nil) != tc.allowed {
					t.Fatalf("with release=%t: rejection=%+v; want allowed=%t", withRelease, rejection, tc.allowed)
				}
			}
		})
	}
}

func explicitUnknownOnly() *qualityprofiles.QualityProfile {
	return &qualityprofiles.QualityProfile{
		ID: "prof-1", Name: "Unknown only",
		Items: `[{"id":"unknown","name":"unknown","allowed":true},{"id":"webdl-720p","name":"webdl-720p","allowed":false}]`,
	}
}

// stubProfileStore is a test double for QualityProfileGetter.
type stubProfileStore struct {
	profile *qualityprofiles.QualityProfile
	err     error
}

func (s *stubProfileStore) Get(_ context.Context, _ string) (*qualityprofiles.QualityProfile, error) {
	return s.profile, s.err
}

// anyProfile returns a profile with upgrade_allowed and a ranked item list.
func anyProfile() *qualityprofiles.QualityProfile {
	return &qualityprofiles.QualityProfile{
		ID:             "prof-1",
		Name:           "HD-1080p",
		UpgradeAllowed: true,
		Cutoff:         "bluray-1080p",
		Items:          `[{"id":"bluray-1080p-remux","name":"bluray-1080p-remux","allowed":true},{"id":"bluray-1080p","name":"bluray-1080p","allowed":true},{"id":"webrip-1080p","name":"webrip-1080p","allowed":true},{"id":"webdl-1080p","name":"webdl-1080p","allowed":true},{"id":"hdtv-1080p","name":"hdtv-1080p","allowed":true}]`,
	}
}

func noUpgradeProfile() *qualityprofiles.QualityProfile {
	p := anyProfile()
	p.UpgradeAllowed = false
	return p
}

func TestUpgradeSpec_NoExistingFile_Allows(t *testing.T) {
	t.Parallel()
	spec := NewUpgradeSpec(&stubProfileStore{profile: anyProfile()})
	c := &ImportCandidate{
		SourcePath:       "Movie.2024.1080p.BluRay.mkv",
		QualityProfileID: "prof-1",
		ExistingQuality:  "", // no existing file
		IncomingRelease:  parser.Parse("Movie.2024.1080p.BluRay.mkv"),
	}
	if r := spec.IsSatisfiedBy(context.Background(), c); r != nil {
		t.Fatalf("expected allow when no existing file, got rejection: %s", r.Message)
	}
}

func TestUpgradeSpec_HigherQuality_Allows(t *testing.T) {
	t.Parallel()
	spec := NewUpgradeSpec(&stubProfileStore{profile: anyProfile()})
	c := &ImportCandidate{
		SourcePath:       "Movie.2024.1080p.BluRay.mkv",
		QualityProfileID: "prof-1",
		ExistingQuality:  "hdtv-1080p", // rank 0
		IncomingRelease:  &parser.Release{Source: "BluRay", Resolution: 1080},
	}
	if r := spec.IsSatisfiedBy(context.Background(), c); r != nil {
		t.Fatalf("expected allow for upgrade, got rejection: %s", r.Message)
	}
}

func TestUpgradeSpec_SameQuality_Allows(t *testing.T) {
	t.Parallel()
	spec := NewUpgradeSpec(&stubProfileStore{profile: anyProfile()})
	c := &ImportCandidate{
		SourcePath:       "Movie.2024.1080p.HDTV.mkv",
		QualityProfileID: "prof-1",
		ExistingQuality:  "hdtv-1080p",
		IncomingRelease:  &parser.Release{Source: "HDTV", Resolution: 1080},
	}
	if r := spec.IsSatisfiedBy(context.Background(), c); r != nil {
		t.Fatalf("expected same-quality replacement, got %v", r)
	}
}

func TestUpgradeSpec_LowerQuality_Rejects(t *testing.T) {
	t.Parallel()
	spec := NewUpgradeSpec(&stubProfileStore{profile: anyProfile()})
	c := &ImportCandidate{
		SourcePath:       "Movie.2024.1080p.HDTV.mkv",
		QualityProfileID: "prof-1",
		ExistingQuality:  "bluray-1080p",                                    // rank 3
		IncomingRelease:  &parser.Release{Source: "HDTV", Resolution: 1080}, // rank 0
	}
	r := spec.IsSatisfiedBy(context.Background(), c)
	if r == nil {
		t.Fatal("expected rejection for lower quality, got allow")
	}
	if r.Reason != RejectionDowngrade {
		t.Fatalf("expected reason %q, got %q", RejectionDowngrade, r.Reason)
	}
}

func TestUpgradeSpec_ProperSameQuality_Allows(t *testing.T) {
	t.Parallel()
	spec := NewUpgradeSpec(&stubProfileStore{profile: anyProfile()})
	c := &ImportCandidate{
		SourcePath:       "Movie.2024.1080p.HDTV.PROPER.mkv",
		QualityProfileID: "prof-1",
		ExistingQuality:  "hdtv-1080p",
		IncomingRelease:  &parser.Release{Source: "HDTV", Resolution: 1080, IsProper: true},
	}
	if r := spec.IsSatisfiedBy(context.Background(), c); r != nil {
		t.Fatalf("expected allow for PROPER of same quality, got rejection: %s", r.Message)
	}
}

func TestUpgradeSpec_RepackSameQuality_Allows(t *testing.T) {
	t.Parallel()
	spec := NewUpgradeSpec(&stubProfileStore{profile: anyProfile()})
	c := &ImportCandidate{
		SourcePath:       "Movie.2024.1080p.HDTV.REPACK.mkv",
		QualityProfileID: "prof-1",
		ExistingQuality:  "hdtv-1080p",
		IncomingRelease:  &parser.Release{Source: "HDTV", Resolution: 1080, IsRepack: true},
	}
	if r := spec.IsSatisfiedBy(context.Background(), c); r != nil {
		t.Fatalf("expected allow for REPACK of same quality, got rejection: %s", r.Message)
	}
}

func TestUpgradeSpec_UpgradesDisabled_Warns(t *testing.T) {
	t.Parallel()
	spec := NewUpgradeSpec(&stubProfileStore{profile: noUpgradeProfile()})
	c := &ImportCandidate{
		SourcePath:       "Movie.2024.1080p.BluRay.mkv",
		QualityProfileID: "prof-1",
		ExistingQuality:  "hdtv-1080p",
		IncomingRelease:  &parser.Release{Source: "BluRay", Resolution: 1080},
	}
	if r := spec.IsSatisfiedBy(context.Background(), c); r != nil || len(c.QualityWarnings) != 1 {
		t.Fatalf("upgrades disabled: rejection=%+v warnings=%v", r, c.QualityWarnings)
	}
}

func TestUpgradeSpec_NoProfile_Allows(t *testing.T) {
	t.Parallel()
	spec := NewUpgradeSpec(&stubProfileStore{profile: anyProfile()})
	c := &ImportCandidate{
		SourcePath:       "Movie.2024.1080p.BluRay.mkv",
		QualityProfileID: "", // no profile assigned
		ExistingQuality:  "hdtv-1080p",
		IncomingRelease:  &parser.Release{Source: "BluRay", Resolution: 1080},
	}

	if r := spec.IsSatisfiedBy(context.Background(), c); r != nil {
		t.Fatalf("expected allow with no profile ID, got rejection: %s", r.Message)
	}
}

func TestUpgradeSpec_ProfileDisallowedIncomingOnlyWarns(t *testing.T) {
	profile := anyProfile()
	c := &ImportCandidate{SourcePath: "Movie.2024.720p.HDTV.mkv", QualityProfileID: profile.ID}
	if r := NewUpgradeSpec(&stubProfileStore{profile: profile}).IsSatisfiedBy(context.Background(), c); r != nil || len(c.QualityWarnings) != 1 {
		t.Fatalf("disallowed incoming quality: rejection=%+v warnings=%v", r, c.QualityWarnings)
	}
}

func TestUpgradeSpec_EqualQualityAllowedWhenUpgradesDisabled(t *testing.T) {
	c := &ImportCandidate{SourcePath: "Movie.2024.1080p.HDTV.mkv", QualityProfileID: "prof-1", ExistingQuality: "hdtv-1080p"}
	if r := NewUpgradeSpec(&stubProfileStore{profile: noUpgradeProfile()}).IsSatisfiedBy(context.Background(), c); r != nil {
		t.Fatalf("expected equal quality to be allowed: %v", r)
	}

}

func TestUpgradeSpec_RejectsDowngradeFromQualityOmittedByProfile(t *testing.T) {
	c := &ImportCandidate{
		SourcePath:       "Movie.2024.1080p.BluRay.mkv",
		QualityProfileID: "prof-1",
		ExistingQuality:  "bluray-2160p",
	}
	if r := NewUpgradeSpec(&stubProfileStore{profile: anyProfile()}).IsSatisfiedBy(context.Background(), c); r == nil {
		t.Fatal("omitting the existing quality from the profile must not permit a downgrade")
	}
	c.SourcePath = "Movie.mkv"
	c.IncomingRelease = parser.Parse("Movie.2024.1080p.BluRay")
	for _, profileID := range []string{"prof-1", ""} {
		c.QualityProfileID = profileID
		if r := NewUpgradeSpec(&stubProfileStore{profile: anyProfile()}).IsSatisfiedBy(context.Background(), c); r == nil {
			t.Fatal("losing filename quality must not bypass a release-context downgrade check")
		}
	}
}

func TestImportUsesSearchQualityDecision(t *testing.T) {
	const title = "Chicago PD S14E06 1080p HEVC x265-MeGusta EZTV.mkv"
	spec := NewUpgradeSpec(&stubProfileStore{profile: anyProfile()})

	// Not grabbed by search: import runs the same detector and assumption.
	c := &ImportCandidate{SourcePath: title, QualityProfileID: "prof-1", IncomingRelease: parser.Parse(title)}
	if r := spec.IsSatisfiedBy(context.Background(), c); r != nil || c.ResolvedTier != "webdl-1080p" {
		t.Fatalf("detected import: rejection=%+v stored=%q", r, c.ResolvedTier)
	}

	// Grabbed by search: the tag is reused without detecting again.
	c = &ImportCandidate{SourcePath: "renamed.mkv", QualityProfileID: "prof-1", GrabbedQuality: "hdtv-1080p"}
	if r := spec.IsSatisfiedBy(context.Background(), c); r != nil || c.ResolvedTier != "hdtv-1080p" || len(c.QualityWarnings) != 0 {
		t.Fatalf("grabbed import: rejection=%+v stored=%q warnings=%v", r, c.ResolvedTier, c.QualityWarnings)
	}
	// A quality the profile does not allow only warns.
	c = &ImportCandidate{SourcePath: title, QualityProfileID: "prof-1", GrabbedQuality: "webdl-720p"}
	if r := spec.IsSatisfiedBy(context.Background(), c); r != nil || len(c.QualityWarnings) != 1 || c.ResolvedTier != "webdl-720p" {
		t.Fatalf("disallowed: rejection=%+v stored=%q warnings=%v", r, c.ResolvedTier, c.QualityWarnings)
	}
}

func TestDowngradeNeedsApproval(t *testing.T) {
	const title = "Chicago PD S14E06 1080p HEVC x265-MeGusta EZTV.mkv"
	spec := NewUpgradeSpec(&stubProfileStore{profile: anyProfile()})
	c := &ImportCandidate{SourcePath: title, QualityProfileID: "prof-1", GrabbedQuality: "webdl-1080p", ExistingQuality: "bluray-1080p"}
	if r := spec.IsSatisfiedBy(context.Background(), c); r == nil || r.Reason != RejectionDowngrade {
		t.Fatalf("downgrade: rejection=%+v", r)
	}
	c = &ImportCandidate{SourcePath: title, QualityProfileID: "prof-1", GrabbedQuality: "webdl-1080p", ExistingQuality: "bluray-1080p", DowngradeApproved: true}
	if r := spec.IsSatisfiedBy(context.Background(), c); r != nil || len(c.QualityWarnings) != 1 || c.ResolvedTier != "webdl-1080p" {
		t.Fatalf("approved downgrade: rejection=%+v stored=%q warnings=%v", r, c.ResolvedTier, c.QualityWarnings)
	}
	// Without a profile, filename evidence still reveals a downgrade.
	c = &ImportCandidate{SourcePath: "Show.S01E01.720p.WEB-DL.mkv", ExistingQuality: "webdl-1080p"}
	if r := spec.IsSatisfiedBy(context.Background(), c); r == nil || r.Reason != RejectionDowngrade {
		t.Fatalf("no profile downgrade: rejection=%+v", r)
	}
}

func TestManualGrabOverridesQualityProfile(t *testing.T) {
	profile := &qualityprofiles.QualityProfile{ID: "prof-1", Name: "HD-1080p", Cutoff: "bluray-1080p", UpgradeAllowed: true,
		Items: `[{"id":"bluray-1080p","name":"bluray-1080p","allowed":true},{"id":"webdl-1080p","name":"webdl-1080p","allowed":true},{"id":"hdtv-720p","name":"hdtv-720p","allowed":false}]`}
	spec := NewUpgradeSpec(&stubProfileStore{profile: profile})
	newCandidate := func(file string, manual bool) *ImportCandidate {
		c := &ImportCandidate{SourcePath: file, QualityProfileID: "prof-1", GrabbedManually: manual}
		c.IncomingRelease = parser.Parse(file)
		return c
	}

	// Any import of a disallowed quality goes ahead with a warning.
	if c := newCandidate("Show.S01E01.720p.HDTV.x264-GRP.mkv", false); spec.IsSatisfiedBy(context.Background(), c) != nil || len(c.QualityWarnings) == 0 {
		t.Fatalf("ungrabbed disallowed quality: warnings=%v", c.QualityWarnings)
	}
	// A manual grab imports it and records why it falls outside the profile.
	c := newCandidate("Show.S01E01.720p.HDTV.x264-GRP.mkv", true)
	if r := spec.IsSatisfiedBy(context.Background(), c); r != nil || c.ResolvedTier != "hdtv-720p" || len(c.QualityWarnings) == 0 {
		t.Fatalf("manual disallowed: rejection=%+v stored=%q warnings=%v", r, c.ResolvedTier, c.QualityWarnings)
	}
	// An allowed manual grab below the cutoff is highlighted.
	c = newCandidate("Show.S01E01.1080p.WEB-DL.x264-GRP.mkv", true)
	if r := spec.IsSatisfiedBy(context.Background(), c); r != nil || len(c.QualityWarnings) != 1 || !strings.Contains(c.QualityWarnings[0], "cutoff") {
		t.Fatalf("manual below cutoff: rejection=%+v warnings=%v", r, c.QualityWarnings)
	}
	// Meeting the cutoff needs no warning.
	c = newCandidate("Show.S01E01.1080p.BluRay.x264-GRP.mkv", true)
	if r := spec.IsSatisfiedBy(context.Background(), c); r != nil || len(c.QualityWarnings) != 0 {
		t.Fatalf("manual at cutoff: rejection=%+v warnings=%v", r, c.QualityWarnings)
	}
}

func TestImportMatchesDVDDefinition(t *testing.T) {
	profile := &qualityprofiles.QualityProfile{ID: "prof-1", Name: "SD",
		Items: `[{"id":"dvd","name":"dvd","allowed":true}]`}
	c := &ImportCandidate{SourcePath: "Movie.1999.DVDRip.XviD-GROUP.avi", QualityProfileID: "prof-1"}
	c.IncomingRelease = parser.Parse(c.SourcePath)
	c.IncomingRelease.Resolution = 480
	if r := NewUpgradeSpec(&stubProfileStore{profile: profile}).IsSatisfiedBy(context.Background(), c); r != nil || c.ResolvedTier != "dvd" {
		t.Fatalf("dvd import: rejection=%+v stored=%q", r, c.ResolvedTier)
	}
}

func TestGrabbedQualityYieldsToFileEvidence(t *testing.T) {
	for _, tc := range []struct {
		file, grabbed, detected string
		want                    bool
	}{
		{"unknown-1080p", "webdl-1080p", "unknown-1080p", true},
		{"webrip-1080p", "webdl-1080p", "unknown-1080p", false},
		{"unknown-720p", "webdl-1080p", "unknown-1080p", false},
		{"bluray-1080p-remux", "bluray-1080p", "bluray-1080p", false},
		{"unknown-1080p", "custom-tier:ct", "unknown-1080p", true},
	} {
		if got := grabbedQualityFits(tc.file, tc.grabbed, tc.detected); got != tc.want {
			t.Errorf("grabbedQualityFits(%q, %q, %q) = %t, want %t", tc.file, tc.grabbed, tc.detected, got, tc.want)
		}
	}
}

func TestUnknownExistingRejectsResolutionDowngrade(t *testing.T) {
	profile := &qualityprofiles.QualityProfile{ID: "prof-1", Name: "Mixed", UpgradeAllowed: true,
		Items: `[{"id":"webdl-720p","name":"webdl-720p","allowed":true},{"id":"unknown","name":"unknown","allowed":true}]`}
	c := &ImportCandidate{SourcePath: "Show.S01E01.720p.WEB-DL.mkv", QualityProfileID: "prof-1", ExistingQuality: "unknown-1080p"}
	c.IncomingRelease = parser.Parse(c.SourcePath)
	if r := NewUpgradeSpec(&stubProfileStore{profile: profile}).IsSatisfiedBy(context.Background(), c); r == nil {
		t.Fatal("720p must not replace an unknown-source 1080p file")
	}
}
