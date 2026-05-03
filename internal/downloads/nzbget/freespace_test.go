package nzbget

import (
	"context"
	"testing"
)

func TestFreeSpace_FromStatusMB(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.on("status", map[string]any{"FreeDiskSpaceMB": float64(2048)})

	c := newTestClient(t, f)
	got, err := c.FreeSpace(context.Background())
	if err != nil {
		t.Fatalf("FreeSpace: %v", err)
	}
	want := int64(2048) * 1024 * 1024
	if got != want {
		t.Fatalf("FreeSpace = %d want %d", got, want)
	}
}

func TestFreeSpace_MissingFieldReturnsNegOne(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.on("status", map[string]any{"DownloadedSizeMB": float64(10)})

	c := newTestClient(t, f)
	got, err := c.FreeSpace(context.Background())
	if err != nil {
		t.Fatalf("FreeSpace: %v", err)
	}
	if got != -1 {
		t.Fatalf("FreeSpace = %d want -1", got)
	}
}
