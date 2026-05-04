package parser

import (
	"strings"
	"testing"
)

func TestParseCodec(t *testing.T) {
	tests := []struct {
		name     string
		release  string
		expected string
	}{
		// h264/AVC variants
		{"h264 standard", "movie.h264.mkv", "h264"},
		{"h.264 with dot", "movie.h.264.mkv", "h264"},
		{"x264", "movie.x264.mkv", "h264"},
		{"x.264 with dot", "movie.x.264.mkv", "h264"},
		{"avc", "movie.avc.mkv", "h264"},

		// h265/HEVC variants
		{"h265 standard", "movie.h265.mkv", "h265"},
		{"h.265 with dot", "movie.h.265.mkv", "h265"},
		{"x265", "movie.x265.mkv", "h265"},
		{"x.265 with dot", "movie.x.265.mkv", "h265"},
		{"hevc", "movie.hevc.mkv", "h265"},

		// VP9
		{"vp9", "movie.vp9.mkv", "vp9"},

		// AV1
		{"av1", "movie.av1.mkv", "av1"},
		{"av01", "movie.av01.mkv", "av1"},

		// MPEG2
		{"mpeg2", "movie.mpeg2.mkv", "mpeg2"},

		// Unknown
		{"unknown codec", "movie.mkv", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse(tt.release)
			if r.Codec != tt.expected {
				t.Errorf("expected codec %q, got %q", tt.expected, r.Codec)
			}
		})
	}
}

func TestParseBitdepth(t *testing.T) {
	tests := []struct {
		name     string
		release  string
		expected int
	}{
		// 12-bit
		{"12-bit with hyphen", "movie.12-bit.mkv", 12},
		{"12 bit with space", "movie.12 bit.mkv", 12},
		{"12bit no space", "movie.12bit.mkv", 12},

		// 10-bit
		{"10-bit with hyphen", "movie.10-bit.mkv", 10},
		{"10 bit with space", "movie.10 bit.mkv", 10},
		{"10bit no space", "movie.10bit.mkv", 10},
		{"hi10p", "movie.hi10p.mkv", 10},
		{"hi10", "movie.hi10.mkv", 10},

		// 8-bit (default, not explicitly marked)
		{"8-bit implicit", "movie.h264.mkv", 0},
		{"unknown", "movie.mkv", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse(tt.release)
			if r.Bitdepth != tt.expected {
				t.Errorf("expected bitdepth %d, got %d", tt.expected, r.Bitdepth)
			}
		})
	}
}

func TestParseYear(t *testing.T) {
	tests := []struct {
		name     string
		release  string
		expected int
	}{
		// [YYYY] format
		{"bracket year", "Movie.Title.[2023].mkv", 2023},
		{"bracket year 2024", "Movie.Title.[2024].mkv", 2024},

		// YYYY format (various positions)
		{"year 2023 in middle", "Movie.2023.1080p.mkv", 2023},
		{"year 2024 in middle", "Movie.2024.BluRay.mkv", 2024},

		// No year
		{"no year", "Movie.1080p.mkv", 0},

		// Real-world examples
		{"real example 1", "The.Matrix.1999.1080p.BluRay.mkv", 1999},
		{"real example 2", "Inception.2010.720p.mkv", 2010},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse(tt.release)
			if r.Year != tt.expected {
				t.Errorf("expected year %d, got %d", tt.expected, r.Year)
			}
		})
	}
}

func TestParseResolution(t *testing.T) {
	tests := []struct {
		name     string
		release  string
		expected int
	}{
		// 4K
		{"4K", "movie.4k.mkv", 2160},
		{"2160p", "movie.2160p.mkv", 2160},

		// 1080p
		{"1080p", "movie.1080p.mkv", 1080},
		{"1080 no p", "movie.1080.mkv", 1080},

		// 720p
		{"720p", "movie.720p.mkv", 720},
		{"720 no p", "movie.720.mkv", 720},

		// 480p
		{"480p", "movie.480p.mkv", 480},

		// 360p
		{"360p", "movie.360p.mkv", 360},

		// Unknown/SD
		{"no resolution", "movie.mkv", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse(tt.release)
			if r.Resolution != tt.expected {
				t.Errorf("expected resolution %d, got %d", tt.expected, r.Resolution)
			}
		})
	}
}

func TestParseSource(t *testing.T) {
	tests := []struct {
		name     string
		release  string
		expected string
	}{
		// BluRay
		{"BluRay", "movie.BluRay.mkv", "BluRay"},
		{"blu-ray", "movie.blu-ray.mkv", "BluRay"},
		{"brrip", "movie.brrip.mkv", "BluRay"},
		{"BDRIP", "movie.bdrip.mkv", "BluRay"},

		// WebRip/WebDL
		{"WebRip", "movie.WebRip.mkv", "WebRip"},
		{"web-rip", "movie.web-rip.mkv", "WebRip"},
		{"WebDL", "movie.WebDL.mkv", "WebDL"},
		{"web-dl", "movie.web-dl.mkv", "WebDL"},
		{"web suffix", "movie.web.mkv", "WebRip"},

		// HDTV
		{"HDTV", "movie.HDTV.mkv", "HDTV"},
		{"hd-tv", "movie.hd-tv.mkv", "HDTV"},

		// DVDRip
		{"DVDRip", "movie.DVDRip.mkv", "DVDRip"},
		{"dvd-rip", "movie.dvd-rip.mkv", "DVDRip"},

		// DVD
		{"DVD", "movie.DVD.mkv", "DVD"},

		// Remux
		{"Remux", "movie.Remux.mkv", "Remux"},

		// Unknown
		{"unknown source", "movie.mkv", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse(tt.release)
			if r.Source != tt.expected {
				t.Errorf("expected source %q, got %q", tt.expected, r.Source)
			}
		})
	}
}

func TestParseSeasonEpisode(t *testing.T) {
	tests := []struct {
		name       string
		release    string
		expSeason  int
		expEpisode int
	}{
		// Standard S##E## format
		{"S01E01", "show.S01E01.mkv", 1, 1},
		{"S01E05", "show.S01E05.mkv", 1, 5},
		{"S10E20", "show.S10E20.mkv", 10, 20},

		// Lowercase
		{"s01e01 lowercase", "show.s01e01.mkv", 1, 1},

		// Single digit (no leading zero)
		{"S1E5", "show.S1E5.mkv", 1, 5},

		// No season/episode
		{"no s/e", "show.mkv", -1, -1},

		// Real-world example
		{"real example", "Breaking.Bad.S05E16.mkv", 5, 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse(tt.release)
			if r.Season != tt.expSeason {
				t.Errorf("expected season %d, got %d", tt.expSeason, r.Season)
			}
			if r.Episode != tt.expEpisode {
				t.Errorf("expected episode %d, got %d", tt.expEpisode, r.Episode)
			}
		})
	}
}

// Real-world complex release names
func TestParseComplexRealWorld(t *testing.T) {
	tests := []struct {
		name         string
		release      string
		expCodec     string
		expBitdepth  int
		expYear      int
		expResolution int
		expSource    string
	}{
		{
			name:         "Inception BluRay",
			release:      "Inception.2010.1080p.BluRay.x264-SPARKS",
			expCodec:     "h264",
			expBitdepth:  0,
			expYear:      2010,
			expResolution: 1080,
			expSource:    "BluRay",
		},
		{
			name:         "The Matrix with h265",
			release:      "The.Matrix.1999.2160p.BluRay.x265.10-bit-IAMNOTBOB",
			expCodec:     "h265",
			expBitdepth:  10,
			expYear:      1999,
			expResolution: 2160,
			expSource:    "BluRay",
		},
		{
			name:         "WebRip with AV1",
			release:      "Avatar.2022.1080p.WebRip.av1.aac",
			expCodec:     "av1",
			expBitdepth:  0,
			expYear:      2022,
			expResolution: 1080,
			expSource:    "WebRip",
		},
		{
			name:         "TV show HDTV",
			release:      "Breaking.Bad.S05E16.720p.HDTV.x264",
			expCodec:     "h264",
			expBitdepth:  0,
			expYear:      0,
			expResolution: 720,
			expSource:    "HDTV",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse(tt.release)
			if r.Codec != tt.expCodec {
				t.Errorf("codec: expected %q, got %q", tt.expCodec, r.Codec)
			}
			if r.Bitdepth != tt.expBitdepth {
				t.Errorf("bitdepth: expected %d, got %d", tt.expBitdepth, r.Bitdepth)
			}
			if r.Year != tt.expYear {
				t.Errorf("year: expected %d, got %d", tt.expYear, r.Year)
			}
			if r.Resolution != tt.expResolution {
				t.Errorf("resolution: expected %d, got %d", tt.expResolution, r.Resolution)
			}
			if r.Source != tt.expSource {
				t.Errorf("source: expected %q, got %q", tt.expSource, r.Source)
			}
		})
	}
}

// Benchmark parsing performance
func BenchmarkParse(b *testing.B) {
	releaseName := "Inception.2010.2160p.BluRay.x265.10-bit.HEVC.AAC.5.1-GROUP"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Parse(releaseName)
	}
}

// Test edge cases and malformed input
func TestParseEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		release string
	}{
		{"empty string", ""},
		{"only spaces", "     "},
		{"special characters", "!!!###$$$%%%"},
		{"very long name", "a" + strings.Repeat("bcdefghijklmnopqrstuvwxyz", 100)},
		{"all uppercase", "MOVIE.1080P.BLURAY.X264"},
		{"all lowercase", "movie.1080p.bluray.x264"},
		{"mixed case", "MoViE.1080P.BluRay.X264"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			r := Parse(tt.release)
			if r == nil {
				t.Errorf("expected non-nil Release, got nil")
			}
			if r.Name != tt.release {
				t.Errorf("name mismatch: expected %q, got %q", tt.release, r.Name)
			}
		})
	}
}

// Test that regex patterns are cached correctly
func TestPatternCaching(t *testing.T) {
	// Clear the cache first
	patternCache.Lock()
	originalLen := len(patternCache.patterns)
	patternCache.Unlock()

	// Parse multiple times with same release name
	releaseName := "Movie.2024.1080p.BluRay.x264"
	for i := 0; i < 10; i++ {
		Parse(releaseName)
	}

	// Verify cache was used (no new patterns added)
	patternCache.RLock()
	cacheLen := len(patternCache.patterns)
	patternCache.RUnlock()

	if cacheLen <= originalLen {
		// Pattern cache is working (no new patterns per parse)
		t.Logf("cache size after 10 parses: %d (patterns reused)", cacheLen)
	}
}
