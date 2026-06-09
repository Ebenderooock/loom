package musicsearch

import (
	"log/slog"
	"testing"

	"github.com/ebenderooock/loom/internal/customformats"
	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/music"
)

func TestBuildAudioQueries(t *testing.T) {
	qs := buildAudioQueries("Pink Floyd", "Animals", 1977)
	if len(qs) != 3 {
		t.Fatalf("want 3 queries, got %d", len(qs))
	}
	if qs[0].Term != "Pink Floyd Animals" {
		t.Errorf("tier0 = %q", qs[0].Term)
	}
	if qs[1].Term != "Pink Floyd Animals 1977" {
		t.Errorf("tier1 = %q", qs[1].Term)
	}
	if qs[2].Term != "Animals" {
		t.Errorf("tier2 = %q", qs[2].Term)
	}
	for _, q := range qs {
		if q.Mode != indexers.ModeSearch {
			t.Errorf("mode = %q, want search", q.Mode)
		}
		if len(q.Categories) == 0 || q.Categories[0] != indexers.CategoryAudio {
			t.Errorf("categories = %v", q.Categories)
		}
	}
}

func TestBuildAudioQueriesNoYear(t *testing.T) {
	qs := buildAudioQueries("Artist", "Album", 0)
	if len(qs) != 2 {
		t.Fatalf("want 2 queries without year, got %d", len(qs))
	}
}

func TestIdentityMatch(t *testing.T) {
	tests := []struct {
		title  string
		artist string
		album  string
		want   bool
	}{
		{"Pink Floyd - Animals (1977) [FLAC]", "Pink Floyd", "Animals", true},
		{"Pink.Floyd-Animals-1977-FLAC", "Pink Floyd", "Animals", true},
		{"Led Zeppelin - IV [FLAC]", "Pink Floyd", "Animals", false},
		{"Pink Floyd - The Wall [FLAC]", "Pink Floyd", "Animals", false},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			parsed := music.ParseMusic(tt.title)
			got := identityMatch(tt.title, parsed, normalize(tt.artist), normalize(tt.album))
			if got != tt.want {
				t.Errorf("identityMatch = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAlbumYear(t *testing.T) {
	tests := []struct {
		date string
		want int
	}{
		{"1977-01-23", 1977},
		{"2020", 2020},
		{"", 0},
		{"unknown", 0},
	}
	for _, tt := range tests {
		got := albumYear(&music.Album{ReleaseDate: tt.date})
		if got != tt.want {
			t.Errorf("albumYear(%q) = %d, want %d", tt.date, got, tt.want)
		}
	}
}

func TestInferProtocol(t *testing.T) {
	seeders := 5
	torrent := &indexers.Result{Seeders: &seeders}
	if inferProtocol(torrent) != downloads.ProtocolTorrent {
		t.Error("expected torrent for seeded result")
	}
	usenet := &indexers.Result{Link: "https://x/file.nzb"}
	if inferProtocol(usenet) != downloads.ProtocolUsenet {
		t.Error("expected usenet for .nzb link")
	}
}

func TestBuildDownloadRequest(t *testing.T) {
	req := buildDownloadRequest(&indexers.Result{
		Title:    "Artist - Album [FLAC]",
		Infohash: "0123456789abcdef0123456789abcdef01234567",
	})
	if req.Magnet == "" {
		t.Error("expected magnet built from infohash")
	}
	if req.Title == "" {
		t.Error("expected title carried through")
	}
}

func TestNormalize(t *testing.T) {
	if got := normalize("Pink Floyd!"); got != "pinkfloyd" {
		t.Errorf("normalize = %q", got)
	}
}

func TestFormatScore(t *testing.T) {
	cf := customformats.CustomFormat{
		ID:   "cf_flac",
		Name: "Prefer FLAC",
		Specifications: []customformats.Specification{
			{Implementation: customformats.ImplAudio, Fields: map[string]any{"value": "FLAC"}},
		},
	}
	eng := NewEngine(nil, nil, nil, slog.Default())
	eng.SetCustomFormats(customformats.NewEngine([]customformats.CustomFormat{cf}))

	profile := &music.AudioQualityProfile{
		FormatItems:    []music.AudioFormatItem{{FormatID: "cf_flac", Score: 50}},
		MinFormatScore: 10,
	}

	// FLAC release matches → score 50.
	flac := &music.MusicRelease{Format: "FLAC"}
	res := indexers.Result{Title: "Artist - Album 2020 FLAC", IndexerID: "x"}
	score, matches := eng.formatScore(profile, flac, &res)
	if score != 50 {
		t.Fatalf("FLAC score = %d, want 50", score)
	}
	if len(matches) != 1 || matches[0].CustomFormatID != "cf_flac" {
		t.Fatalf("matches = %+v, want one cf_flac match", matches)
	}

	// MP3 release does not match → score 0.
	mp3 := &music.MusicRelease{Format: "MP3"}
	res2 := indexers.Result{Title: "Artist - Album 2020 MP3 320", IndexerID: "x"}
	if s, _ := eng.formatScore(profile, mp3, &res2); s != 0 {
		t.Fatalf("MP3 score = %d, want 0", s)
	}
}

func TestFormatScoreNoEngine(t *testing.T) {
	eng := NewEngine(nil, nil, nil, slog.Default())
	rel := &music.MusicRelease{Format: "FLAC"}
	res := indexers.Result{Title: "x FLAC", IndexerID: "x"}
	if s, m := eng.formatScore(nil, rel, &res); s != 0 || m != nil {
		t.Fatalf("no-engine score = %d matches=%v, want 0/nil", s, m)
	}
}
