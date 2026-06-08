package mediafiles

import "testing"

func TestIsAudio(t *testing.T) {
	audio := []string{".flac", ".MP3", ".m4a", ".ogg", ".opus", ".wav", ".aiff"}
	for _, ext := range audio {
		if !IsAudio(ext) {
			t.Errorf("IsAudio(%q) = false, want true", ext)
		}
	}

	notAudio := []string{".mkv", ".mp4", ".txt", ".nfo", ".jpg", ""}
	for _, ext := range notAudio {
		if IsAudio(ext) {
			t.Errorf("IsAudio(%q) = true, want false", ext)
		}
	}
}

func TestIsAudioNotVideo(t *testing.T) {
	// Audio and video extension sets must not overlap.
	for ext := range AudioExtensions {
		if VideoExtensions[ext] {
			t.Errorf("extension %q is in both AudioExtensions and VideoExtensions", ext)
		}
	}
}
