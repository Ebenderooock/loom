package qualityprofiles

import (
	"testing"

	"github.com/ebenderooock/loom/internal/parser"
)

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
