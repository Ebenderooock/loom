package imports

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/ebenderooock/loom/internal/downloads"
)

func openReplacementDestination(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	// Delete-sharing does not guarantee MoveFileEx can replace an open target.
	handle, err := windows.CreateFile(name, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}

func readReplacementDestination(path string) ([]byte, error) {
	file, err := openReplacementDestination(path)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(file)
	return data, errors.Join(readErr, file.Close())
}

func replacementBlockedByOpenReader(err error, dest, stagedName string) bool {
	var renameErr *os.LinkError
	return errors.As(err, &renameErr) &&
		renameErr.Op == "rename" && renameErr.New == dest &&
		filepath.Base(renameErr.Old) == stagedName &&
		(errors.Is(renameErr.Err, windows.ERROR_ACCESS_DENIED) ||
			errors.Is(renameErr.Err, windows.ERROR_SHARING_VIOLATION))
}

func TestReplacementBlockedByWindowsReaderPreservesBoth(t *testing.T) {
	for _, tc := range []struct {
		name string
		open func(string) (*os.File, error)
	}{
		{"Go", os.Open},
		{"shared", openReplacementDestination},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, source, dest, match := replacementFixture(t, "Movie.1080p.BluRay.mkv", "hdtv-1080p", []byte("complete new media"))
			reader, err := tc.open(dest)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = reader.Close() })
			old, err := reader.Stat()
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.importMatchedWith(context.Background(), &downloads.DownloadCompletedEvent{Title: "Movie"}, match, source, filepath.Base(dest), true)
			if !replacementBlockedByOpenReader(err, dest, "incoming") {
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
			for _, path := range []string{source, dest} {
				if data, err := os.ReadFile(path); err != nil || string(data) != "complete new media" {
					t.Fatalf("replacement contents at %s: %q, %v", path, data, err)
				}
			}
		})
	}
}
