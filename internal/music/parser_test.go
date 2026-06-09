package music

import "testing"

func TestParseMusic(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		artist   string
		album    string
		year     int
		format   string
		bitrate  int
		vbr      bool
		vbrTag   string
		lossless bool
		bits     int
		media    string
	}{
		{
			name:     "flac with year bracket",
			title:    "Pink Floyd - The Dark Side of the Moon (1973) [FLAC]",
			artist:   "Pink Floyd",
			album:    "The Dark Side of the Moon",
			year:     1973,
			format:   "FLAC",
			lossless: true,
		},
		{
			name:    "mp3 320",
			title:   "Artist - Album (2020) [MP3 320]",
			artist:  "Artist",
			album:   "Album",
			year:    2020,
			format:  "MP3",
			bitrate: 320,
		},
		{
			name:   "mp3 vbr v0 multi dash",
			title:  "VA - Greatest Hits - 2020 - MP3-V0",
			artist: "VA",
			album:  "Greatest Hits",
			year:   2020,
			format: "MP3",
			vbr:    true,
			vbrTag: "V0",
		},
		{
			name:     "24bit flac web",
			title:    "Miles Davis - Kind of Blue - 1959 [24bit FLAC] WEB",
			artist:   "Miles Davis",
			album:    "Kind of Blue",
			year:     1959,
			format:   "FLAC",
			lossless: true,
			bits:     24,
			media:    "WEB",
		},
		{
			name:    "kbps suffix",
			title:   "Radiohead - OK Computer 320kbps MP3",
			artist:  "Radiohead",
			format:  "MP3",
			bitrate: 320,
		},
		{
			name:     "aac 256",
			title:    "Some Artist - Some Album [AAC 256]",
			artist:   "Some Artist",
			album:    "Some Album",
			format:   "AAC",
			bitrate:  256,
			lossless: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ParseMusic(tt.title)
			if tt.artist != "" && r.Artist != tt.artist {
				t.Errorf("Artist = %q, want %q", r.Artist, tt.artist)
			}
			if tt.album != "" && r.Album != tt.album {
				t.Errorf("Album = %q, want %q", r.Album, tt.album)
			}
			if tt.year != 0 && r.Year != tt.year {
				t.Errorf("Year = %d, want %d", r.Year, tt.year)
			}
			if tt.format != "" && r.Format != tt.format {
				t.Errorf("Format = %q, want %q", r.Format, tt.format)
			}
			if tt.bitrate != 0 && r.Bitrate != tt.bitrate {
				t.Errorf("Bitrate = %d, want %d", r.Bitrate, tt.bitrate)
			}
			if r.VBR != tt.vbr {
				t.Errorf("VBR = %v, want %v", r.VBR, tt.vbr)
			}
			if tt.vbrTag != "" && r.VBRTag != tt.vbrTag {
				t.Errorf("VBRTag = %q, want %q", r.VBRTag, tt.vbrTag)
			}
			if r.Lossless != tt.lossless {
				t.Errorf("Lossless = %v, want %v", r.Lossless, tt.lossless)
			}
			if tt.bits != 0 && r.Bits != tt.bits {
				t.Errorf("Bits = %d, want %d", r.Bits, tt.bits)
			}
			if tt.media != "" && r.Media != tt.media {
				t.Errorf("Media = %q, want %q", r.Media, tt.media)
			}
		})
	}
}

func TestParseMusicLosslessNoBitrate(t *testing.T) {
	r := ParseMusic("Artist - Album [FLAC 24bit]")
	if r.Bitrate != 0 {
		t.Errorf("lossless Bitrate = %d, want 0", r.Bitrate)
	}
	if !r.Lossless {
		t.Error("expected lossless")
	}
}
