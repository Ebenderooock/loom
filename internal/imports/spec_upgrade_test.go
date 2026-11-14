package imports

import (
	"context"
	"encoding/json"
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

func TestUnknownQualityDoesNotBypassExclusionsOrReplaceKnownQuality(t *testing.T) {
	for _, tc := range []struct {
		name     string
		profile  *qualityprofiles.QualityProfile
		existing string
		allowed  bool
	}{
		{"unrestricted first import", unrestrictedProfile(t), "", true},
		{"restricted first import", anyProfile(), "", false},
		{"unknown replacement", unrestrictedProfile(t), "bluray-1080p", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &ImportCandidate{SourcePath: "Episode 1 Title.mkv", QualityProfileID: tc.profile.ID, ExistingQuality: tc.existing}
			rejection := NewUpgradeSpec(&stubProfileStore{profile: tc.profile}).IsSatisfiedBy(context.Background(), c)
			if (rejection == nil) != tc.allowed {
				t.Fatalf("unknown-quality decision: %+v; allow=%v", rejection, tc.allowed)
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
	if r := NewUpgradeSpec(&stubProfileStore{profile: profile}).IsSatisfiedBy(context.Background(), c); r == nil {
		t.Fatal("profile named Any with an explicit exclusion must reject unknown quality")
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
	if r.Reason != RejectionNotUpgrade {
		t.Fatalf("expected reason %q, got %q", RejectionNotUpgrade, r.Reason)
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

func TestUpgradeSpec_UpgradesDisabled_Rejects(t *testing.T) {
	t.Parallel()
	spec := NewUpgradeSpec(&stubProfileStore{profile: noUpgradeProfile()})
	c := &ImportCandidate{
		SourcePath:       "Movie.2024.1080p.BluRay.mkv",
		QualityProfileID: "prof-1",
		ExistingQuality:  "hdtv-1080p",
		IncomingRelease:  &parser.Release{Source: "BluRay", Resolution: 1080},
	}
	r := spec.IsSatisfiedBy(context.Background(), c)
	if r == nil {
		t.Fatal("expected rejection when upgrades disabled, got allow")
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

func TestUpgradeSpec_ProfileDisallowsIncomingEvenWithoutExisting(t *testing.T) {
	profile := anyProfile()
	c := &ImportCandidate{SourcePath: "Movie.2024.720p.HDTV.mkv", QualityProfileID: profile.ID}
	if r := NewUpgradeSpec(&stubProfileStore{profile: profile}).IsSatisfiedBy(context.Background(), c); r == nil {
		t.Fatal("expected disallowed incoming quality to be rejected")
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
