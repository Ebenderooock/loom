package imports

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanMetadataDoesNotReadLargeCopiedPayloads(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	root := t.TempDir()
	src := filepath.Join(root, "Supernatural.S01E01.mkv")
	dest := filepath.Join(t.TempDir(), "library.mkv")
	for _, path := range []string{src, dest} {
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		// Sparse files make an accidental full-read scan exceed the deadline
		// without requiring actual NAS traffic or allocating the payload.
		err = file.Truncate(32 << 30)
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("create sparse media: %v, %v", err, closeErr)
		}
	}
	ctx := context.Background()
	if err := f.p.recordStatus(ctx, "episode", "ep1", src, dest, StatusImported, ""); err != nil {
		t.Fatal(err)
	}
	scanCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	results, err := f.p.ScanFolder(scanCtx, root)
	if err != nil || len(results) != 1 || results[0].SuggestedAction != "previously_imported" {
		t.Fatalf("metadata scan read payloads or lost history: %+v, %v", results, err)
	}
}

func TestScanMetadataInvalidationAndHardlinkIdentity(t *testing.T) {
	cases := []struct {
		name string
		edit func(*testing.T, string, string)
		hint bool
	}{
		{"unchanged copy", nil, true},
		{"source changed", func(t *testing.T, src, _ string) {
			if err := os.WriteFile(src, []byte("different size"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"destination changed", func(t *testing.T, _, dest string) {
			info, err := os.Stat(dest)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(dest, info.ModTime(), info.ModTime().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"destination missing", func(t *testing.T, _, dest string) {
			if err := os.Remove(dest); err != nil {
				t.Fatal(err)
			}
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newPackPipeline(t, ImportModeCopy, nil)
			root, paths := writePack(t, t.TempDir(), 1)
			src, dest := paths[0], filepath.Join(t.TempDir(), "copy.mkv")
			if err := copyFile(src, dest); err != nil {
				t.Fatal(err)
			}
			if err := f.p.recordStatus(context.Background(), "episode", "ep1", src, dest, StatusImported, ""); err != nil {
				t.Fatal(err)
			}
			if tc.edit != nil {
				tc.edit(t, src, dest)
			}
			results, err := f.p.ScanFolder(context.Background(), root)
			if err != nil || len(results) != 1 {
				t.Fatalf("scan: %+v, %v", results, err)
			}
			if (results[0].SuggestedAction == "previously_imported") != tc.hint {
				t.Fatalf("metadata hint: %+v", results[0])
			}
		})
	}
	t.Run("legacy hardlink", func(t *testing.T) {
		f := newPackPipeline(t, ImportModeHardlink, nil)
		root, files := writePack(t, t.TempDir(), 1)
		dest := filepath.Join(t.TempDir(), "link.mkv")
		if err := os.Link(files[0], dest); err != nil {
			t.Fatal(err)
		}
		insertHistory(t, f.p, files[0], dest, 0, 0, time.Now())
		results, err := f.p.ScanFolder(context.Background(), root)
		if err != nil || len(results) != 1 || results[0].SuggestedAction != "already_imported" {
			t.Fatalf("hardlink identity was not recognized: %+v, %v", results, err)
		}
	})
}

func TestHistoricalMetadataCannotAuthorizeSkippingOrDeletingChangedBytes(t *testing.T) {
	f := newPackPipeline(t, ImportModeMove, nil)
	root, files := writePack(t, t.TempDir(), 1)
	src, dest := files[0], filepath.Join(t.TempDir(), "copy.mkv")
	if err := copyFile(src, dest); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := f.p.recordStatus(ctx, "episode", "ep1", src, dest, StatusImported, ""); err != nil {
		t.Fatal(err)
	}
	if previous, err := f.p.completedImport(ctx, src); previous != dest || err != nil {
		t.Fatalf("initial verification: %q, %v", previous, err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	// Replace equal-size bytes and restore mtime to demonstrate why metadata
	// hints and cached content comparisons cannot authorize deletion.
	if err := os.WriteFile(dest, []byte("corrupt!!"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dest, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	results, err := f.p.ScanFolder(ctx, root)
	if err != nil || len(results) != 1 || results[0].SuggestedAction != "previously_imported" {
		t.Fatalf("expected explicitly historical, unverified metadata hint: %+v, %v", results, err)
	}
	if previous, err := f.p.completedImport(ctx, src); err != nil || previous != "" {
		t.Fatalf("metadata or stale verification authorized skip: %q, %v", previous, err)
	}
	if err := f.p.FinalizeImports(ctx, []string{dest}); err == nil {
		t.Fatal("metadata authorized source deletion")
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source was not preserved: %v", err)
	}
}

func TestLegacyCopyGainsMetadataOnlyAfterContentVerification(t *testing.T) {
	f := newPackPipeline(t, ImportModeCopy, nil)
	root, files := writePack(t, t.TempDir(), 1)
	src, dest := files[0], filepath.Join(t.TempDir(), "copy.mkv")
	if err := copyFile(src, dest); err != nil {
		t.Fatal(err)
	}
	insertHistory(t, f.p, src, dest, 0, 0, time.Now())
	ctx := context.Background()
	results, err := f.p.ScanFolder(ctx, root)
	if err != nil || len(results) != 1 || results[0].SuggestedAction == "previously_imported" {
		t.Fatalf("unverified legacy copy was hidden: %+v, %v", results, err)
	}
	if previous, err := f.p.completedImport(ctx, src); err != nil || previous != dest {
		t.Fatalf("legacy verification: %q, %v", previous, err)
	}
	results, err = f.p.ScanFolder(ctx, root)
	if err != nil || len(results) != 1 || results[0].SuggestedAction != "previously_imported" {
		t.Fatalf("verified legacy metadata was not saved: %+v, %v", results, err)
	}
}
