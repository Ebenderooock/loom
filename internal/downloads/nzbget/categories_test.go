package nzbget

import (
	"context"
	"testing"
	"time"
)

func TestCategories_ParsesConfigSection(t *testing.T) {
	// Tests in this file share package-level state (nowFunc, clientCache); run serially.
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.on("config", []configOption{
		{Name: "MainDir", Value: "/downloads"},
		{Name: "Category1.Name", Value: "movies"},
		{Name: "Category1.DestDir", Value: "/downloads/movies"},
		{Name: "Category2.Name", Value: "tv"},
		{Name: "Category2.DestDir", Value: "/downloads/tv"},
		{Name: "Category3.Name", Value: ""}, // gap
		{Name: "Category4.Name", Value: "books"},
		{Name: "ServerHost", Value: "news.example"},
	})

	c := newTestClient(t, f)
	cats, err := c.Categories(context.Background())
	if err != nil {
		t.Fatalf("Categories: %v", err)
	}
	if len(cats) != 3 {
		t.Fatalf("expected 3 cats, got %d (%v)", len(cats), cats)
	}
	if cats[0].Name != "movies" || cats[0].SavePath != "/downloads/movies" {
		t.Errorf("cats[0] = %+v", cats[0])
	}
	if cats[2].Name != "books" {
		t.Errorf("cats[2] = %+v", cats[2])
	}
}

func TestCategories_Caches(t *testing.T) {
	f := newFakeServer(t, "u", "p")
	defer f.close()
	f.on("config", []configOption{
		{Name: "Category1.Name", Value: "movies"},
	})

	c := newTestClient(t, f)
	if _, err := c.Categories(context.Background()); err != nil {
		t.Fatalf("Categories: %v", err)
	}
	if _, err := c.Categories(context.Background()); err != nil {
		t.Fatalf("Categories (cached): %v", err)
	}
	if got := f.callCount("config"); got != 1 {
		t.Fatalf("expected 1 config call (cache hit), got %d", got)
	}

	// Force expiry — drive the package clock forward past TTL.
	prev := nowFunc
	defer func() { nowFunc = prev }()
	nowFunc = func() time.Time { return prev().Add(2 * categoryCacheTTL) }

	if _, err := c.Categories(context.Background()); err != nil {
		t.Fatalf("Categories after expiry: %v", err)
	}
	if got := f.callCount("config"); got != 2 {
		t.Fatalf("expected 2 config calls after expiry, got %d", got)
	}
}

func TestCategories_KeepsCacheOnTransientError(t *testing.T) {
	f := newFakeServer(t, "u", "p")
	defer f.close()
	calls := 0
	f.onFunc("config", func(_ *testing.T, _ []any) (any, *rpcError, int) {
		calls++
		if calls == 1 {
			return []configOption{{Name: "Category1.Name", Value: "movies"}}, nil, 200
		}
		return nil, &rpcError{Code: -1, Message: "transient"}, 200
	})

	c := newTestClient(t, f)
	if _, err := c.Categories(context.Background()); err != nil {
		t.Fatalf("first call: %v", err)
	}
	prev := nowFunc
	defer func() { nowFunc = prev }()
	nowFunc = func() time.Time { return prev().Add(2 * categoryCacheTTL) }

	cats, err := c.Categories(context.Background())
	if err != nil {
		t.Fatalf("expected cached fallback on error, got %v", err)
	}
	if len(cats) != 1 || cats[0].Name != "movies" {
		t.Fatalf("expected cached movies, got %+v", cats)
	}
}
