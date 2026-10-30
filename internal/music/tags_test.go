package music

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestAudioFormatFromExtensionDoesNotAssumeM4AACodec(t *testing.T) {
	if got := audioFormatFromExt(".m4a"); got != "M4A" {
		t.Fatalf(".m4a format = %q, want generic M4A container", got)
	}
	if got := audioFormatFromExt(".alac"); got != "ALAC" {
		t.Fatalf(".alac format = %q, want ALAC", got)
	}
}

func TestReadFLACBitDepth(t *testing.T) {
	streamInfo := uint64(44100)<<44 | uint64(1)<<41 | uint64(23)<<36
	header := make([]byte, 42)
	copy(header, "fLaC")
	header[4] = 0x80 // Last metadata block, STREAMINFO type.
	header[7] = 34
	binary.BigEndian.PutUint64(header[18:26], streamInfo)

	path := filepath.Join(t.TempDir(), "sample.flac")
	if err := os.WriteFile(path, header, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readFLACBitDepth(path); got != 24 {
		t.Fatalf("readFLACBitDepth() = %d, want 24", got)
	}
}

func TestEstimateAudioBitrate(t *testing.T) {
	if got := EstimateAudioBitrate(4_800_000, 120_000); got != 320 {
		t.Fatalf("EstimateAudioBitrate() = %d, want 320", got)
	}
	if got := EstimateAudioBitrate(1, 0); got != 0 {
		t.Fatalf("EstimateAudioBitrate() with unknown duration = %d, want 0", got)
	}
}
