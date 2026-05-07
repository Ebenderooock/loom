package nzbget

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
)

func TestAdd_NoPayloadReturnsMalformed(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	c := newTestClient(t, f)
	_, err := c.Add(context.Background(), downloads.AddRequest{})
	if !errors.Is(err, ErrMalformedNZB) {
		t.Fatalf("expected ErrMalformedNZB, got %v", err)
	}
}

func TestAdd_RawBytes_Base64AndCategory(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.on("append", float64(4242)) // JSON-RPC numerics decode as float64

	c := newTestClient(t, f)
	body := []byte("<nzb><file>fake</file></nzb>")
	res, err := c.Add(context.Background(), downloads.AddRequest{
		RawBytes: body,
		Title:    "Movie 2024",
		Category: "movies",
		Tags:     []string{"priority=50", "add_paused", "add_to_top", "pp_passwd=hunter2"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.ItemID != "4242" {
		t.Fatalf("ItemID = %q want 4242", res.ItemID)
	}
	params := f.params("append")
	if len(params) != 10 {
		t.Fatalf("expected 10 positional params, got %d: %v", len(params), params)
	}
	// [0] NZBFilename — title is suffixed with .nzb when missing.
	if got := params[0].(string); got != "Movie 2024.nzb" {
		t.Errorf("filename = %q", got)
	}
	// [1] NZBContent — base64 of raw bytes.
	wantContent := base64.StdEncoding.EncodeToString(body)
	if got := params[1].(string); got != wantContent {
		t.Errorf("content mismatch")
	}
	// [2] Category.
	if got := params[2].(string); got != "movies" {
		t.Errorf("category = %q", got)
	}
	// [3] Priority.
	if got := params[3].(float64); int(got) != 50 {
		t.Errorf("priority = %v", got)
	}
	// [4] AddToTop.
	if got := params[4].(bool); !got {
		t.Errorf("add_to_top should be true")
	}
	// [5] AddPaused.
	if got := params[5].(bool); !got {
		t.Errorf("add_paused should be true")
	}
	// [9] PostProcessParameters — slice of [name,value] pairs.
	pp, ok := params[9].([]any)
	if !ok {
		t.Fatalf("post-process params not a list: %T", params[9])
	}
	if len(pp) != 1 {
		t.Fatalf("expected 1 pp entry, got %d", len(pp))
	}
	pair := pp[0].([]any)
	if pair[0].(string) != "passwd" || pair[1].(string) != "hunter2" {
		t.Errorf("pp pair mismatch: %v", pair)
	}
}

func TestAdd_URL_Passthrough(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.on("append", float64(7))

	c := newTestClient(t, f)
	res, err := c.Add(context.Background(), downloads.AddRequest{
		NZBURL:   "https://nzb.example/abc.nzb",
		Category: "tv",
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.ItemID != "7" {
		t.Fatalf("ItemID = %q", res.ItemID)
	}
	params := f.params("append")
	// URL is passed verbatim as NZBFilename; NZBContent stays empty.
	if got := params[0].(string); got != "https://nzb.example/abc.nzb" {
		t.Errorf("filename = %q", got)
	}
	if got := params[1].(string); got != "" {
		t.Errorf("content should be empty for URL adds, got %q", got)
	}
	if got := params[2].(string); got != "tv" {
		t.Errorf("category = %q", got)
	}
}

func TestAdd_DuplicatedReturnsMissingNZBID(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.on("append", float64(0))

	c := newTestClient(t, f)
	_, err := c.Add(context.Background(), downloads.AddRequest{NZBURL: "https://x"})
	if !errors.Is(err, ErrMissingNZBID) {
		t.Fatalf("expected ErrMissingNZBID, got %v", err)
	}
}

func TestAdd_NegativeReturnsServerError(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.on("append", float64(-1))

	c := newTestClient(t, f)
	_, err := c.Add(context.Background(), downloads.AddRequest{NZBURL: "https://x"})
	if !errors.Is(err, ErrServer) {
		t.Fatalf("expected ErrServer, got %v", err)
	}
}

func TestAdd_FilenameSuffixAdded(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.on("append", float64(1))

	c := newTestClient(t, f)
	_, _ = c.Add(context.Background(), downloads.AddRequest{
		RawBytes: []byte("dummy"),
		Title:    "no-suffix",
	})
	got := f.params("append")[0].(string)
	if !strings.HasSuffix(got, ".nzb") {
		t.Errorf("filename should be suffixed with .nzb, got %q", got)
	}
}
