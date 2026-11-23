//go:build !windows

package diskspace

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestVolumeIDUsesStatDeviceAndFollowsSymlinks(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	var stat unix.Stat_t
	if err := unix.Stat(root, &stat); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("dev:%d", stat.Dev)
	got, err := VolumeID(link)
	if err != nil || got != want {
		t.Fatalf("VolumeID(link) = %q, %v; want %q", got, err, want)
	}
}
