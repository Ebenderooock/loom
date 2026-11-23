package diskspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVolumeIDSameFilesystem(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	first, err := VolumeID(root)
	if err != nil || first == "" {
		t.Fatalf("VolumeID(root) = %q, %v", first, err)
	}
	second, err := VolumeID(child)
	if err != nil || second != first {
		t.Fatalf("VolumeID(child) = %q, %v; want %q", second, err, first)
	}
	if _, err := VolumeID(filepath.Join(root, "missing")); err == nil {
		t.Fatal("expected an error for a missing path")
	}
}
