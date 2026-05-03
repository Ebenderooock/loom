package sabnzbd

import (
	"context"
	"testing"
)

func TestFreeSpace_Diskspace1(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	f.on("fullstatus", `{"fullstatus":{"diskspace1":"512.0"}}`)

	c := newTestClient(t, f)
	got, err := c.FreeSpace(context.Background())
	if err != nil {
		t.Fatalf("FreeSpace: %v", err)
	}
	// 512 GB → 512 * 1024 MB → 512 * 1024 * 1024 * 1024 bytes
	want := int64(512) * 1024 * 1024 * 1024
	if got != want {
		t.Fatalf("free space = %d want %d", got, want)
	}
}

func TestFreeSpace_LegacyTopLevel(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	f.on("fullstatus", `{"diskspace1":"100.0"}`)

	c := newTestClient(t, f)
	got, err := c.FreeSpace(context.Background())
	if err != nil {
		t.Fatalf("FreeSpace: %v", err)
	}
	if got != int64(100)*1024*1024*1024 {
		t.Fatalf("got %d", got)
	}
}

func TestFreeSpace_Missing(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	f.on("fullstatus", `{"fullstatus":{}}`)

	c := newTestClient(t, f)
	got, err := c.FreeSpace(context.Background())
	if err != nil {
		t.Fatalf("FreeSpace: %v", err)
	}
	if got != -1 {
		t.Fatalf("expected -1, got %d", got)
	}
}
