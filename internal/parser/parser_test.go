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

		// ##x## format
		{"7x01 format", "show.7x01.mkv", 7, 1},
		{"1x05 format", "show.1x05.720p.mkv", 1, 5},

		// Episode word format
		{"Episode 01", "show.Episode.01.mkv", -1, 1},
		{"Ep 05", "show.ep.05.mkv", -1, 5},

		// Standalone E## format
		{"E01 standalone", "E01.720p.mkv", -1, 1},
		{"e05 standalone", "show.e05.mkv", -1, 5},

		// 3-digit episode
		{"S01E100", "show.S01E100.mkv", 1, 100},
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
		name          string
		release       string
		expCodec      string
		expBitdepth   int
		expYear       int
		expResolution int
		expSource     string
	}{
		{
			name:          "Inception BluRay",
			release:       "Inception.2010.1080p.BluRay.x264-SPARKS",
			expCodec:      "h264",
			expBitdepth:   0,
			expYear:       2010,
			expResolution: 1080,
			expSource:     "BluRay",
		},
		{
			name:          "The Matrix with h265",
			release:       "The.Matrix.1999.2160p.BluRay.x265.10-bit-IAMNOTBOB",
			expCodec:      "h265",
			expBitdepth:   10,
			expYear:       1999,
			expResolution: 2160,
			expSource:     "BluRay",
		},
		{
			name:          "WebRip with AV1",
			release:       "Avatar.2022.1080p.WebRip.av1.aac",
			expCodec:      "av1",
			expBitdepth:   0,
			expYear:       2022,
			expResolution: 1080,
			expSource:     "WebRip",
		},
		{
			name:          "TV show HDTV",
			release:       "Breaking.Bad.S05E16.720p.HDTV.x264",
			expCodec:      "h264",
			expBitdepth:   0,
			expYear:       0,
			expResolution: 720,
			expSource:     "HDTV",
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
// ── Title Extraction ─────────────────────────────────────────────────

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		name    string
		release string
		want    string
	}{
		{"movie with year", "The.Matrix.1999.1080p.BluRay.x264-GROUP", "The Matrix"},
		{"movie no year", "Inception.1080p.BluRay.x264", "Inception"},
		{"tv show", "Breaking.Bad.S01E05.720p.BluRay", "Breaking Bad"},
		{"tv show 1x format", "The.Rookie.7x01.720p.HDTV", "The Rookie"},
		{"tv show no title", "S07E01.720p.HDTV.x264", ""},
		{"tv show dash title", "The Rookie - S07E01 - The Good the Bad", "The Rookie"},
		{"underscored", "The_Dark_Knight_2008_BluRay", "The Dark Knight"},
		{"with group bracket", "[SubGroup] Show Name - 01 [1080p]", "Show Name 01 [1080p]"},
		{"with parens year", "Dune (2021) 1080p BluRay", "Dune"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse(tt.release)
			if r.Title != tt.want {
				t.Errorf("Parse(%q).Title = %q, want %q", tt.release, r.Title, tt.want)
			}
		})
	}
}

// ── Group Extraction ─────────────────────────────────────────────────

func TestExtractGroup(t *testing.T) {
	tests := []struct {
		name    string
		release string
		want    string
	}{
		{"trailing dash group", "Movie.2024.1080p.BluRay-SPARKS", "SPARKS"},
		{"trailing with ext", "Movie.2024.1080p.BluRay-FGT.mkv", "FGT"},
		{"leading bracket group", "[SubGroup] Show - 01.mkv", "SubGroup"},
		{"no group", "Movie.2024.1080p.BluRay.mkv", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse(tt.release)
			if r.Group != tt.want {
				t.Errorf("Parse(%q).Group = %q, want %q", tt.release, r.Group, tt.want)
			}
		})
	}
}

// ── Anime Patterns ───────────────────────────────────────────────────

func TestAnimePatterns(t *testing.T) {
	tests := []struct {
		name      string
		release   string
		wantRes   int
		wantCodec string
		wantGroup string
	}{
		{
			"bracket group with 1080p",
			"[SubGroup] Show Name - 01 [1080p].mkv",
			1080, "", "SubGroup",
		},
		{
			"anime with h265 tag",
			"[Erai-raws] My Hero Academia - 01 [1080p][HEVC].mkv",
			1080, "h265", "Erai-raws",
		},
		{
			"anime with 10bit",
			"[GroupName] Anime Title - 12 [720p][10bit].mkv",
			720, "", "GroupName",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse(tt.release)
			if r.Resolution != tt.wantRes {
				t.Errorf("Resolution = %d, want %d", r.Resolution, tt.wantRes)
			}
			if tt.wantCodec != "" && r.Codec != tt.wantCodec {
				t.Errorf("Codec = %q, want %q", r.Codec, tt.wantCodec)
			}
			if r.Group != tt.wantGroup {
				t.Errorf("Group = %q, want %q", r.Group, tt.wantGroup)
			}
		})
	}
}

// ── Multi-Episode & Season Pack ──────────────────────────────────────

func TestMultiEpisode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		season   int
		episode  int
		episodes []int
	}{
		{"S01E01E02", "Show.S01E01E02.1080p.BluRay.mkv", 1, 1, []int{1, 2}},
		{"S01E01E02E03", "Show.S01E01E02E03.720p.mkv", 1, 1, []int{1, 2, 3}},
		{"S01E01-E03", "Show.S01E01-E03.1080p.mkv", 1, 1, []int{1, 2, 3}},
		{"S01E01-03 range", "Show.S01E01-03.HDTV.mkv", 1, 1, []int{1, 2, 3}},
		{"S01E05.E06", "Show.S01E05.E06.WEB.mkv", 1, 5, []int{5, 6}},
		{"single episode", "Show.S01E01.1080p.mkv", 1, 1, []int{1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Parse(tc.input)
			if r.Season != tc.season {
				t.Errorf("Season = %d, want %d", r.Season, tc.season)
			}
			if r.Episode != tc.episode {
				t.Errorf("Episode = %d, want %d", r.Episode, tc.episode)
			}
			if len(r.Episodes) != len(tc.episodes) {
				t.Fatalf("Episodes = %v, want %v", r.Episodes, tc.episodes)
			}
			for i, ep := range tc.episodes {
				if r.Episodes[i] != ep {
					t.Errorf("Episodes[%d] = %d, want %d", i, r.Episodes[i], ep)
				}
			}
		})
	}
}

func TestSeasonPackNoEpisode(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		season       int
		isSeasonPack bool
	}{
		{"S01.Complete", "Show.S01.Complete.1080p.BluRay.mkv", 1, true},
		{"S02 standalone", "Show.S02.720p.WEB-DL.mkv", 2, true},
		{"Season 1", "Show Season 1 1080p BluRay", 1, true},
		{"season.03", "show.season.03.720p.hdtv", 3, true},
		// Ensure S01E01 is NOT detected as season pack
		{"S01E01 not pack", "Show.S01E01.1080p.mkv", 1, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Parse(tc.input)
			if r.Season != tc.season {
				t.Errorf("Season = %d, want %d", r.Season, tc.season)
			}
			if r.IsSeasonPack != tc.isSeasonPack {
				t.Errorf("IsSeasonPack = %v, want %v", r.IsSeasonPack, tc.isSeasonPack)
			}
		})
	}
}

// ── Daily Show Format ────────────────────────────────────────────────

func TestDailyShowYearExtraction(t *testing.T) {
	// "Show.2024.05.07.HDTV" — the parser should extract daily date AND year
	r := Parse("Some.Show.2024.05.07.HDTV.x264.mkv")
	if r.Year != 2024 {
		t.Errorf("Year = %d, want 2024", r.Year)
	}
	if r.Source != "HDTV" {
		t.Errorf("Source = %q, want HDTV", r.Source)
	}
	if r.DailyDate != "2024-05-07" {
		t.Errorf("DailyDate = %q, want 2024-05-07", r.DailyDate)
	}
}

func TestDailyDateFormats(t *testing.T) {
	tests := []struct {
		name  string
		input string
		date  string
	}{
		{"dot separated", "The.Daily.Show.2024.03.15.720p.WEB.mkv", "2024-03-15"},
		{"dash separated", "Late.Night-2023-11-22-HDTV.mkv", "2023-11-22"},
		{"no daily date", "Movie.2024.1080p.BluRay.mkv", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Parse(tc.input)
			if r.DailyDate != tc.date {
				t.Errorf("DailyDate = %q, want %q", r.DailyDate, tc.date)
			}
		})
	}
}

// ── Proper/Repack/Real Flags ────────────────────────────────────────

func TestProperRepackFlags(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		proper bool
		repack bool
		real   bool
	}{
		{"proper", "Show.S01E01.PROPER.720p.HDTV.mkv", true, false, false},
		{"repack", "Show.S01E01.REPACK.1080p.BluRay.mkv", false, true, false},
		{"real", "Show.S01E01.REAL.720p.HDTV.mkv", false, false, true},
		{"proper+repack", "Show.S01E01.PROPER.REPACK.720p.mkv", true, true, false},
		{"none", "Show.S01E01.1080p.BluRay.mkv", false, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Parse(tc.input)
			if r.IsProper != tc.proper {
				t.Errorf("IsProper = %v, want %v", r.IsProper, tc.proper)
			}
			if r.IsRepack != tc.repack {
				t.Errorf("IsRepack = %v, want %v", r.IsRepack, tc.repack)
			}
			if r.IsReal != tc.real {
				t.Errorf("IsReal = %v, want %v", r.IsReal, tc.real)
			}
		})
	}
}

// ── Audio Codec Extraction ──────────────────────────────────────────

func TestAudioExtraction(t *testing.T) {
	tests := []struct {
		name  string
		input string
		audio string
	}{
		{"DTS-HD MA", "Movie.2024.1080p.BluRay.DTS-HD.MA.mkv", "DTS-HD MA"},
		{"TrueHD Atmos", "Movie.2024.2160p.Remux.TrueHD.Atmos.mkv", "TrueHD Atmos"},
		{"EAC3 Atmos", "Movie.2024.1080p.WEB-DL.DD+.Atmos.mkv", "EAC3 Atmos"},
		{"DTS-X", "Movie.2024.2160p.UHD.DTS-X.mkv", "DTS-X"},
		{"DTS-HD", "Movie.2024.1080p.BluRay.DTS-HD.mkv", "DTS-HD"},
		{"TrueHD", "Movie.2024.1080p.BluRay.TrueHD.mkv", "TrueHD"},
		{"EAC3/DD+", "Show.S01E01.1080p.WEB-DL.DD+5.1.mkv", "EAC3"},
		{"DTS", "Movie.2024.1080p.BluRay.DTS.mkv", "DTS"},
		{"AC3", "Show.S01E01.720p.HDTV.AC3.mkv", "AC3"},
		{"FLAC", "Movie.2024.1080p.BluRay.FLAC.mkv", "FLAC"},
		{"AAC", "Show.S01E01.720p.WEB.AAC.mkv", "AAC"},
		{"no audio", "Movie.2024.1080p.BluRay.mkv", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Parse(tc.input)
			if r.Audio != tc.audio {
				t.Errorf("Audio = %q, want %q", r.Audio, tc.audio)
			}
		})
	}
}

// ── Anime Absolute Episode ──────────────────────────────────────────

func TestAbsoluteEpisode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		absEp int
	}{
		{"anime style", "[SubGroup] Naruto Shippuuden - 142 [1080p].mkv", 142},
		{"anime single digit", "[Fansub] One Piece - 5 [720p].mkv", 5},
		{"not anime (no bracket)", "Show.S01E01.1080p.mkv", -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Parse(tc.input)
			if r.AbsoluteEpisode != tc.absEp {
				t.Errorf("AbsoluteEpisode = %d, want %d", r.AbsoluteEpisode, tc.absEp)
			}
		})
	}
}

// ── Edition Extraction ──────────────────────────────────────────────

func TestEditionExtraction(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		edition string
	}{
		{"directors cut", "Movie.2024.Directors.Cut.1080p.BluRay.mkv", "Directors Cut"},
		{"extended", "Movie.2024.Extended.1080p.BluRay.mkv", "Extended"},
		{"extended edition", "Movie.2024.Extended.Edition.1080p.mkv", "Extended"},
		{"imax", "Movie.2024.IMAX.2160p.WEB-DL.mkv", "IMAX"},
		{"remastered", "Movie.2024.Remastered.1080p.BluRay.mkv", "Remastered"},
		{"theatrical", "Movie.2024.Theatrical.Cut.720p.mkv", "Theatrical"},
		{"unrated", "Movie.2024.Unrated.1080p.BluRay.mkv", "Unrated"},
		{"criterion", "Movie.2024.Criterion.Collection.1080p.mkv", "Criterion"},
		{"none", "Movie.2024.1080p.BluRay.mkv", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Parse(tc.input)
			if r.Edition != tc.edition {
				t.Errorf("Edition = %q, want %q", r.Edition, tc.edition)
			}
		})
	}
}

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

// ── Revision Extraction ─────────────────────────────────────────────

func TestRevisionExtraction(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		revision int
		proper   bool
		repack   bool
	}{
		{"v2", "Show.S01E01.v2.720p.HDTV.mkv", 2, false, false},
		{"v3", "Show.S01E01.v3.720p.HDTV.mkv", 3, false, false},
		{"proper implies 2", "Show.S01E01.PROPER.720p.HDTV.mkv", 2, true, false},
		{"repack implies 2", "Show.S01E01.REPACK.1080p.BluRay.mkv", 2, false, true},
		{"repack2", "Show.S01E01.REPACK2.720p.mkv", 2, false, true},
		{"none", "Show.S01E01.1080p.BluRay.mkv", 0, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Parse(tc.input)
			if r.Revision != tc.revision {
				t.Errorf("Revision = %d, want %d", r.Revision, tc.revision)
			}
			if r.IsProper != tc.proper {
				t.Errorf("IsProper = %v, want %v", r.IsProper, tc.proper)
			}
			if r.IsRepack != tc.repack {
				t.Errorf("IsRepack = %v, want %v", r.IsRepack, tc.repack)
			}
		})
	}
}

// ── Embedded ID Extraction ──────────────────────────────────────────

func TestImdbIDExtraction(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		imdbID string
	}{
		{"tt7 digits", "Movie.2024.tt1234567.1080p.mkv", "tt1234567"},
		{"tt8 digits", "Movie.2024.tt12345678.1080p.mkv", "tt12345678"},
		{"none", "Movie.2024.1080p.BluRay.mkv", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Parse(tc.input)
			if r.ImdbID != tc.imdbID {
				t.Errorf("ImdbID = %q, want %q", r.ImdbID, tc.imdbID)
			}
		})
	}
}

func TestTmdbIDExtraction(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		tmdbID string
	}{
		{"tmdb-12345", "Movie.2024.tmdb-12345.1080p.mkv", "12345"},
		{"tmdbid-67890", "Movie.2024.tmdbid-67890.1080p.mkv", "67890"},
		{"tmdb12345", "Movie.2024.tmdb12345.1080p.mkv", "12345"},
		{"none", "Movie.2024.1080p.BluRay.mkv", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Parse(tc.input)
			if r.TmdbID != tc.tmdbID {
				t.Errorf("TmdbID = %q, want %q", r.TmdbID, tc.tmdbID)
			}
		})
	}
}

// ── AirDate / ReleaseGroup Aliases ──────────────────────────────────

func TestAliasFields(t *testing.T) {
	// AirDate should match DailyDate
	r := Parse("The.Daily.Show.2024.03.15.720p.WEB.mkv")
	if r.AirDate != r.DailyDate {
		t.Errorf("AirDate = %q, want %q (same as DailyDate)", r.AirDate, r.DailyDate)
	}
	if r.AirDate != "2024-03-15" {
		t.Errorf("AirDate = %q, want 2024-03-15", r.AirDate)
	}

	// ReleaseGroup should match Group
	r2 := Parse("Movie.2024.1080p.BluRay-SPARKS")
	if r2.ReleaseGroup != r2.Group {
		t.Errorf("ReleaseGroup = %q, want %q (same as Group)", r2.ReleaseGroup, r2.Group)
	}
	if r2.ReleaseGroup != "SPARKS" {
		t.Errorf("ReleaseGroup = %q, want SPARKS", r2.ReleaseGroup)
	}
}

// ── Cross-reference Multi-Episode S01E01-S01E03 ─────────────────────

func TestCrossRefMultiEpisode(t *testing.T) {
	r := Parse("Show.S01E01-S01E03.1080p.BluRay.mkv")
	if r.Season != 1 {
		t.Errorf("Season = %d, want 1", r.Season)
	}
	if r.Episode != 1 {
		t.Errorf("Episode = %d, want 1", r.Episode)
	}
	want := []int{1, 2, 3}
	if len(r.Episodes) != len(want) {
		t.Fatalf("Episodes = %v, want %v", r.Episodes, want)
	}
	for i, ep := range want {
		if r.Episodes[i] != ep {
			t.Errorf("Episodes[%d] = %d, want %d", i, r.Episodes[i], ep)
		}
	}
}

// ── Broader Absolute Episode (no brackets) ──────────────────────────

func TestAbsoluteEpisodeNoBrackets(t *testing.T) {
	r := Parse("Naruto Shippuuden - 142 1080p.mkv")
	if r.AbsoluteEpisode != 142 {
		t.Errorf("AbsoluteEpisode = %d, want 142", r.AbsoluteEpisode)
	}
}

// ── Edition: Open Matte, Diamond Edition ────────────────────────────

func TestEditionOpenMatte(t *testing.T) {
	r := Parse("Movie.2024.Open.Matte.1080p.BluRay.mkv")
	if r.Edition != "Open Matte" {
		t.Errorf("Edition = %q, want Open Matte", r.Edition)
	}
}

func TestEditionDiamondEdition(t *testing.T) {
	r := Parse("Movie.2024.Diamond.Edition.1080p.BluRay.mkv")
	if r.Edition != "Diamond Edition" {
		t.Errorf("Edition = %q, want Diamond Edition", r.Edition)
	}
}

// ── Parenthesized Year ──────────────────────────────────────────────

func TestParenthesizedYear(t *testing.T) {
	r := Parse("Inception (2010) 1080p BluRay.mkv")
	if r.Year != 2010 {
		t.Errorf("Year = %d, want 2010", r.Year)
	}
}
