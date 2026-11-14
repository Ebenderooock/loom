package qualityprofiles

import (
	"encoding/json"
	"testing"

	"github.com/ebenderooock/loom/internal/parser"
)

func TestAllowsUnknownQualityRequiresUnrestrictedCatalog(t *testing.T) {
	definition := defaultProfileDefinitions()[0]
	items := make([]QualityItem, 0, len(definition.items))
	for _, name := range definition.items {
		items = append(items, QualityItem{ID: name, Name: name, Allowed: true})
	}
	for _, tc := range []struct {
		name    string
		items   []QualityItem
		allowed bool
	}{
		{"all seeded tiers", items, true},
		{"missing tier", items[1:], false},
		{"explicit unknown exclusion", append(append([]QualityItem(nil), items...), QualityItem{Name: "unknown", Allowed: false}), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.items)
			if err != nil {
				t.Fatal(err)
			}
			profile := &QualityProfile{Name: "Any", Items: string(raw)}
			if got := profile.AllowsUnknownQuality(); got != tc.allowed {
				t.Fatalf("unknown quality allowed=%v, want=%v", got, tc.allowed)
			}
		})
	}
}

func TestQualityProfile_Rank(t *testing.T) {
	t.Parallel()
	p := &QualityProfile{
		Items: `[{"id":"hdtv-1080p","name":"hdtv-1080p","allowed":true},{"id":"webdl-1080p","name":"webdl-1080p","allowed":true},{"id":"bluray-1080p","name":"bluray-1080p","allowed":true}]`,
	}

	tests := []struct {
		name     string
		quality  string
		wantRank int
	}{
		{"first item", "hdtv-1080p", 0},
		{"second item", "webdl-1080p", 1},
		{"last item", "bluray-1080p", 2},
		{"unknown", "dvd-480p", -1},
		{"case insensitive", "HDTV-1080p", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.Rank(tt.quality)
			if got != tt.wantRank {
				t.Errorf("Rank(%q) = %d, want %d", tt.quality, got, tt.wantRank)
			}
		})
	}
}

func TestQualityNameFromRelease(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		rel  *parser.Release
		want string
	}{
		{"bluray 1080p", &parser.Release{Source: "BluRay", Resolution: 1080}, "bluray-1080p"},
		{"webdl 720p", &parser.Release{Source: "WebDL", Resolution: 720}, "webdl-720p"},
		{"hdtv 2160p", &parser.Release{Source: "HDTV", Resolution: 2160}, "hdtv-2160p"},
		{"remux", &parser.Release{Source: "BluRay", Resolution: 2160, IsRemux: true}, "bluray-2160p-remux"},
		{"empty", &parser.Release{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QualityNameFromRelease(tt.rel)
			if got != tt.want {
				t.Errorf("QualityNameFromRelease() = %q, want %q", got, tt.want)
			}
		})
	}
}
