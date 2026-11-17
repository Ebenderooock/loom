package imports

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/ebenderooock/loom/internal/downloads"
)

func readReplacementDestination(path string) ([]byte, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	// Go's os.Open omits FILE_SHARE_DELETE, which prevents replacement while
	// the handle is open. This reader explicitly permits rename/delete.
	handle, err := windows.CreateFile(name, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	data, readErr := io.ReadAll(file)
	return data, errors.Join(readErr, file.Close())
}

func TestReplacementBlockedByWindowsReaderPreservesBoth(t *testing.T) {
	p, source, dest, match := replacementFixture(t, "Movie.1080p.BluRay.mkv", "hdtv-1080p", []byte("complete new media"))
	reader, err := os.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	old, err := reader.Stat()
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.importMatchedWith(context.Background(), &downloads.DownloadCompletedEvent{Title: "Movie"}, match, source, filepath.Base(dest), true)
	if err == nil || !strings.Contains(err.Error(), "publish replacement") {
		t.Fatalf("expected blocked publication, got %v", err)
	}
	for path, want := range map[string]string{source: "complete new media", dest: "old media"} {
		if data, err := os.ReadFile(path); err != nil || string(data) != want {
			t.Fatalf("file %s: %q, %v", path, data, err)
		}
	}
	current, err := os.Stat(dest)
	if err != nil || !os.SameFile(old, current) {
		t.Fatalf("blocked replacement changed the live file: %v", err)
	}
	var quality string
	var size int64
	if err := p.db.QueryRow(`SELECT quality, size FROM movie_files`).Scan(&quality, &size); err != nil || quality != "hdtv-1080p" || size != 9 {
		t.Fatalf("blocked replacement changed registration: quality=%q size=%d err=%v", quality, size, err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.importMatchedWith(context.Background(), &downloads.DownloadCompletedEvent{Title: "Movie"}, match, source, filepath.Base(dest), true); err != nil {
		t.Fatalf("replacement after reader closed: %v", err)
	}
	if data, err := os.ReadFile(dest); err != nil || string(data) != "complete new media" {
		t.Fatalf("replacement contents: %q, %v", data, err)
	}
}
