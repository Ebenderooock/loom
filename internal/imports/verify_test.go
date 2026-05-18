package imports

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerify_ValidFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := filepath.Join(dir, "movie.mkv")
	data := []byte("fake media content for testing")
	if err := os.WriteFile(f, data, 0o644); err != nil {
		t.Fatal(err)
	}

	v := &ImportVerifier{}
	res := v.Verify(f, int64(len(data)))
	if !res.OK {
		t.Fatalf("expected OK, got reason: %s", res.Reason)
	}
}

func TestVerify_MissingFile(t *testing.T) {
	t.Parallel()
	v := &ImportVerifier{}
	res := v.Verify("/nonexistent/path/movie.mkv", 1000)
	if res.OK {
		t.Fatal("expected verification to fail for missing file")
	}
	if res.Reason == "" {
		t.Fatal("expected a reason for failure")
	}
}

func TestVerify_WrongSize(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := filepath.Join(dir, "movie.mkv")
	if err := os.WriteFile(f, []byte("short"), 0o644); err != nil {
		t.Fatal(err)
	}

	v := &ImportVerifier{}
	res := v.Verify(f, 999999)
	if res.OK {
		t.Fatal("expected verification to fail for size mismatch")
	}
	if res.Reason == "" {
		t.Fatal("expected a reason for failure")
	}
}

func TestVerify_ZeroByte(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := filepath.Join(dir, "movie.mkv")
	if err := os.WriteFile(f, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	v := &ImportVerifier{}
	res := v.Verify(f, 0)
	if res.OK {
		t.Fatal("expected verification to fail for zero-byte file")
	}
}

func TestVerify_SkipSizeCheck_WhenExpectedZero(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := filepath.Join(dir, "movie.mkv")
	if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	v := &ImportVerifier{}
	// expectedSize=0 means "don't check size", but file is non-zero so OK
	res := v.Verify(f, 0)
	if !res.OK {
		t.Fatalf("expected OK when expectedSize is 0, got reason: %s", res.Reason)
	}
}
