package music

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"
)

// AudioTags holds the metadata extracted from an audio file's embedded tags.
type AudioTags struct {
	Artist      string
	AlbumArtist string
	Album       string
	Title       string
	TrackNumber int
	TrackTotal  int
	DiscNumber  int
	DiscTotal   int
	Year        int
	Genre       string
	Format      string // normalized container/codec, e.g. "FLAC", "MP3"
}

// ReadAudioTags reads embedded metadata from the audio file at path using the
// pure-Go dhowden/tag library (no CGO, no external binaries). Missing tags are
// returned as zero values rather than errors so callers can fall back to
// filename/path heuristics.
func ReadAudioTags(path string) (*AudioTags, error) {
	f, err := os.Open(path) //nolint:gosec // path comes from a scanned library folder
	if err != nil {
		return nil, fmt.Errorf("open audio file: %w", err)
	}
	defer func() { _ = f.Close() }()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, fmt.Errorf("read tags: %w", err)
	}

	trackNum, trackTotal := m.Track()
	discNum, discTotal := m.Disc()

	t := &AudioTags{
		Artist:      strings.TrimSpace(m.Artist()),
		AlbumArtist: strings.TrimSpace(m.AlbumArtist()),
		Album:       strings.TrimSpace(m.Album()),
		Title:       strings.TrimSpace(m.Title()),
		TrackNumber: trackNum,
		TrackTotal:  trackTotal,
		DiscNumber:  discNum,
		DiscTotal:   discTotal,
		Year:        m.Year(),
		Genre:       strings.TrimSpace(m.Genre()),
		Format:      audioFormatFromFileType(m.FileType()),
	}
	if t.Format == "" {
		t.Format = audioFormatFromExt(filepath.Ext(path))
	}
	return t, nil
}

// audioFormatFromFileType maps a dhowden/tag FileType to a human label.
func audioFormatFromFileType(ft tag.FileType) string {
	switch ft {
	case tag.MP3:
		return "MP3"
	case tag.M4A:
		return "M4A"
	case tag.M4B:
		return "M4B"
	case tag.M4P:
		return "M4P"
	case tag.ALAC:
		return "ALAC"
	case tag.FLAC:
		return "FLAC"
	case tag.OGG:
		return "OGG"
	case tag.DSF:
		return "DSF"
	default:
		return ""
	}
}

// audioFormatFromExt derives a format label from the file extension when the
// embedded tags don't identify the container.
func audioFormatFromExt(ext string) string {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "flac":
		return "FLAC"
	case "mp3":
		return "MP3"
	case "m4a", "alac":
		return "M4A"
	case "m4b":
		return "M4B"
	case "aac":
		return "AAC"
	case "ogg", "oga":
		return "OGG"
	case "opus":
		return "Opus"
	case "wav":
		return "WAV"
	case "wv":
		return "WavPack"
	case "ape":
		return "APE"
	case "aiff", "aif":
		return "AIFF"
	case "wma":
		return "WMA"
	default:
		return strings.ToUpper(strings.TrimPrefix(ext, "."))
	}
}
