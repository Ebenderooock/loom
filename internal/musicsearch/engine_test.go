package musicsearch

import (
	"testing"

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
