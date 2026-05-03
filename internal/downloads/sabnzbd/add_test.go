package sabnzbd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/loomctl/loom/internal/downloads"
)

func TestAdd_NoPayloadReturnsMalformed(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	c := newTestClient(t, f)
	_, err := c.Add(context.Background(), downloads.AddRequest{})
	if !errors.Is(err, ErrMalformedNZB) {
		t.Fatalf("expected ErrMalformedNZB, got %v", err)
	}
}

func TestAdd_URL_PassesCategoryAndPriority(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	f.on("addurl", `{"status":true,"nzo_ids":["SAB-1234"]}`)

	c := newTestClient(t, f)
	res, err := c.Add(context.Background(), downloads.AddRequest{
		NZBURL:   "https://nzb.example/abc.nzb",
		Title:    "Test Movie 2024",
		Category: "movies",
		Tags:     []string{"priority=1", "script=postproc.sh", "pp=3"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.ItemID != "SAB-1234" {
		t.Fatalf("ItemID = %q want SAB-1234", res.ItemID)
	}

	form := f.lastForm["addurl"]
	if form.Get("name") != "https://nzb.example/abc.nzb" {
		t.Errorf("name = %q", form.Get("name"))
	}
	if form.Get("cat") != "movies" {
		t.Errorf("cat = %q", form.Get("cat"))
	}
	if form.Get("priority") != "1" {
		t.Errorf("priority = %q", form.Get("priority"))
	}
	if form.Get("script") != "postproc.sh" {
		t.Errorf("script = %q", form.Get("script"))
	}
	if form.Get("pp") != "3" {
		t.Errorf("pp = %q", form.Get("pp"))
	}
	if form.Get("nzbname") != "Test Movie 2024" {
		t.Errorf("nzbname = %q", form.Get("nzbname"))
	}
}

func TestAdd_File_Multipart(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	f.on("addfile", `{"status":true,"nzo_ids":["SAB-FILE"]}`)

	c := newTestClient(t, f)
	body := []byte("<nzb><file>fake</file></nzb>")
	res, err := c.Add(context.Background(), downloads.AddRequest{
		RawBytes: body,
		Title:    "Movie",
		Category: "tv",
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.ItemID != "SAB-FILE" {
		t.Fatalf("got %q", res.ItemID)
	}
	form := f.lastForm["addfile"]
	if form.Get("cat") != "tv" {
		t.Errorf("cat = %q", form.Get("cat"))
	}
	if form.Get("nzbname") != "Movie" {
		t.Errorf("nzbname = %q", form.Get("nzbname"))
	}
}

func TestAdd_File_FilenameSuffix(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	f.on("addfile", `{"status":true,"nzo_ids":["X"]}`)

	c := newTestClient(t, f)
	_, _ = c.Add(context.Background(), downloads.AddRequest{
		RawBytes: []byte("dummy"),
		Title:    "no-suffix",
	})
	// nzbname should not have ".nzb" forced onto it; only the
	// multipart filename does. The fake captures the field value.
	if got := f.lastForm["addfile"].Get("nzbname"); got != "no-suffix" {
		t.Errorf("nzbname should keep operator-supplied title, got %q", got)
	}
}

func TestAdd_EmptyNzoIDsIsServerError(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	f.on("addurl", `{"status":true,"nzo_ids":[]}`)

	c := newTestClient(t, f)
	_, err := c.Add(context.Background(), downloads.AddRequest{NZBURL: "https://x"})
	if !errors.Is(err, ErrServer) {
		t.Fatalf("expected ErrServer, got %v", err)
	}
	if !strings.Contains(err.Error(), "no nzo_id") {
		t.Errorf("missing context: %v", err)
	}
}
