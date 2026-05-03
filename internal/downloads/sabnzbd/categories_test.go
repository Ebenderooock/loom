package sabnzbd

import (
	"context"
	"testing"
)

func TestCategories_FlatList(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	// get_config refuses; flat list returns names with the SAB
	// "*" sentinel that must be filtered out.
	f.on("get_config", `{"status":false,"error":"locked"}`)
	f.on("get_cats", `{"categories":["*","movies","tv","books"]}`)

	c := newTestClient(t, f)
	cats, err := c.Categories(context.Background())
	if err != nil {
		t.Fatalf("Categories: %v", err)
	}
	if len(cats) != 3 {
		t.Fatalf("expected 3 categories, got %d (%v)", len(cats), cats)
	}
	for i, want := range []string{"movies", "tv", "books"} {
		if cats[i].Name != want {
			t.Errorf("[%d] name = %q want %q", i, cats[i].Name, want)
		}
		if cats[i].SavePath != "" {
			t.Errorf("[%d] flat list should have empty SavePath, got %q", i, cats[i].SavePath)
		}
	}
}

func TestCategories_RichConfig(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t, "k")
	defer f.close()
	f.on("get_config", `{
        "config": {
            "categories": [
                {"name": "*", "dir": ""},
                {"name": "movies", "dir": "/data/movies"},
                {"name": "tv", "dir": "/data/tv"}
            ]
        }
    }`)

	c := newTestClient(t, f)
	cats, err := c.Categories(context.Background())
	if err != nil {
		t.Fatalf("Categories: %v", err)
	}
	if len(cats) != 2 {
		t.Fatalf("got %d cats: %+v", len(cats), cats)
	}
	if cats[0].Name != "movies" || cats[0].SavePath != "/data/movies" {
		t.Errorf("rich[0] = %+v", cats[0])
	}
	if cats[1].SavePath != "/data/tv" {
		t.Errorf("rich[1] = %+v", cats[1])
	}
}
