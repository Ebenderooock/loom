package cardigann

import (
	"testing"
)

// TestJSONNavigate verifies dot-path navigation into nested maps.
func TestJSONNavigate(t *testing.T) {
	root := map[string]any{
		"data": map[string]any{
			"movies": []any{
				map[string]any{"title": "Movie 1"},
				map[string]any{"title": "Movie 2"},
			},
			"movie_count": float64(2),
		},
	}

	// Navigate to nested object
	movies := jsonNavigate(root, "data.movies")
	arr, ok := toSlice(movies)
	if !ok || len(arr) != 2 {
		t.Fatalf("expected 2 movies, got %v", movies)
	}

	// Navigate to scalar
	count := jsonNavigate(root, "data.movie_count")
	if count != float64(2) {
		t.Fatalf("expected 2, got %v", count)
	}

	// Empty path returns root
	got := jsonNavigate(root, "")
	if got == nil {
		t.Fatal("empty path should return root")
	}

	// Missing path returns nil
	got = jsonNavigate(root, "data.nonexistent")
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

// TestJSONValueToString converts various JSON types to strings.
func TestJSONValueToString(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"int_float", float64(42), "42"},
		{"fractional", 3.14, "3.14"},
		{"bool_true", true, "true"},
		{"bool_false", false, "false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonValueToString(tt.val)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestJSONFieldValue tests parent (..) and direct field access.
func TestJSONFieldValue(t *testing.T) {
	child := map[string]any{
		"quality": "720p",
		"hash":    "abc123",
		"seeds":   float64(10),
	}
	parent := map[string]any{
		"title": "The Running Man",
		"year":  float64(2025),
	}

	// Direct field
	if got := jsonFieldValue("quality", child, parent); got != "720p" {
		t.Errorf("expected 720p, got %q", got)
	}

	// Parent access with ..
	if got := jsonFieldValue("..title", child, parent); got != "The Running Man" {
		t.Errorf("expected The Running Man, got %q", got)
	}

	// Parent access for numeric
	if got := jsonFieldValue("..year", child, parent); got != "2025" {
		t.Errorf("expected 2025, got %q", got)
	}

	// Parent access with nil parent falls back to child
	if got := jsonFieldValue("..quality", child, nil); got != "720p" {
		t.Errorf("expected 720p from child fallback, got %q", got)
	}
}

// TestApplyCaseMap verifies the case mapping logic.
func TestApplyCaseMap(t *testing.T) {
	caseMap := map[string]string{
		"720p":  "45",
		"1080p": "44",
		"*":     "99",
	}
	if got := applyCaseMap("720p", caseMap); got != "45" {
		t.Errorf("expected 45, got %q", got)
	}
	if got := applyCaseMap("unknown", caseMap); got != "99" {
		t.Errorf("expected 99 (default), got %q", got)
	}
}

// TestExtractRowsJSON_YTSStyle tests the full JSON extraction with
// YTS-style attribute expansion (movies × torrents).
func TestExtractRowsJSON_YTSStyle(t *testing.T) {
	def := &Definition{
		ID:    "test-yts",
		Name:  "Test YTS",
		Links: []string{"https://example.com"},
		Search: Search{
			Rows: RowsBlock{
				Selector:                        "data.movies",
				Attribute:                       "torrents",
				Multiple:                        true,
				MissingAttributeEqualsNoResults: true,
			},
			Fields: map[string]Field{
				"title":                {Selector: "..title"},
				"quality":              {Selector: "quality"},
				"infohash":             {Selector: "hash"},
				"seeders":              {Selector: "seeds"},
				"leechers":             {Selector: "peers"},
				"size":                 {Selector: "size_bytes"},
				"downloadvolumefactor": {Text: "0"},
			},
		},
	}

	e := &Engine{id: "test-yts", def: def}

	body := []byte(`{
		"status": "ok",
		"data": {
			"movie_count": 2,
			"movies": [
				{
					"title": "The Running Man",
					"year": 2025,
					"url": "https://example.com/movie/1",
					"torrents": [
						{
							"hash": "AAAA1111",
							"quality": "720p",
							"seeds": 100,
							"peers": 50,
							"size_bytes": 1073741824
						},
						{
							"hash": "BBBB2222",
							"quality": "1080p",
							"seeds": 200,
							"peers": 75,
							"size_bytes": 2147483648
						}
					]
				},
				{
					"title": "Another Movie",
					"year": 2024,
					"url": "https://example.com/movie/2",
					"torrents": [
						{
							"hash": "CCCC3333",
							"quality": "2160p",
							"seeds": 5,
							"peers": 2,
							"size_bytes": 4294967296
						}
					]
				}
			]
		}
	}`)

	tctx := templateContext{
		Config: map[string]string{},
		Result: map[string]string{},
	}

	results, err := e.extractRowsJSON(body, tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results (2+1 torrents), got %d", len(results))
	}

	// First result: The Running Man × 720p
	if results[0].Title != "The Running Man" {
		t.Errorf("result[0].Title = %q, want %q", results[0].Title, "The Running Man")
	}
	if results[0].Infohash != "AAAA1111" {
		t.Errorf("result[0].Infohash = %q, want AAAA1111", results[0].Infohash)
	}
	if results[0].Size != 1073741824 {
		t.Errorf("result[0].Size = %d, want 1073741824", results[0].Size)
	}
	if results[0].Seeders == nil || *results[0].Seeders != 100 {
		t.Errorf("result[0].Seeders = %v, want 100", results[0].Seeders)
	}
	if !results[0].Freeleech {
		t.Error("result[0].Freeleech should be true (downloadvolumefactor=0)")
	}

	// Second result: The Running Man × 1080p
	if results[1].Title != "The Running Man" {
		t.Errorf("result[1].Title = %q, want %q", results[1].Title, "The Running Man")
	}
	if results[1].Infohash != "BBBB2222" {
		t.Errorf("result[1].Infohash = %q, want BBBB2222", results[1].Infohash)
	}

	// Third result: Another Movie × 2160p
	if results[2].Title != "Another Movie" {
		t.Errorf("result[2].Title = %q, want %q", results[2].Title, "Another Movie")
	}
	if results[2].Infohash != "CCCC3333" {
		t.Errorf("result[2].Infohash = %q, want CCCC3333", results[2].Infohash)
	}
}

// TestExtractRowsJSON_FlatArray tests JSON extraction for a flat
// array response (like The Pirate Bay).
func TestExtractRowsJSON_FlatArray(t *testing.T) {
	def := &Definition{
		ID:    "test-tpb",
		Name:  "Test TPB",
		Links: []string{"https://example.com"},
		Search: Search{
			Rows: RowsBlock{
				Selector: "$",
			},
			Fields: map[string]Field{
				"title":                {Selector: "name"},
				"infohash":             {Selector: "info_hash"},
				"seeders":              {Selector: "seeders"},
				"leechers":             {Selector: "leechers"},
				"size":                 {Selector: "size"},
				"downloadvolumefactor": {Text: "0"},
			},
		},
	}

	e := &Engine{id: "test-tpb", def: def}

	body := []byte(`[
		{
			"id": 1,
			"name": "Ubuntu 24.04 Desktop",
			"info_hash": "DEADBEEF",
			"seeders": 500,
			"leechers": 100,
			"size": 5368709120
		},
		{
			"id": 2,
			"name": "Fedora 40 Server",
			"info_hash": "CAFEBABE",
			"seeders": 200,
			"leechers": 50,
			"size": 2147483648
		}
	]`)

	tctx := templateContext{
		Config: map[string]string{},
		Result: map[string]string{},
	}

	results, err := e.extractRowsJSON(body, tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].Title != "Ubuntu 24.04 Desktop" {
		t.Errorf("result[0].Title = %q, want Ubuntu 24.04 Desktop", results[0].Title)
	}
	if results[0].Infohash != "DEADBEEF" {
		t.Errorf("result[0].Infohash = %q, want DEADBEEF", results[0].Infohash)
	}
	if results[0].Seeders == nil || *results[0].Seeders != 500 {
		t.Errorf("result[0].Seeders = %v, want 500", results[0].Seeders)
	}
}

// TestExtractRowsJSON_MissingAttribute tests that
// missingAttributeEqualsNoResults skips rows without the attribute.
func TestExtractRowsJSON_MissingAttribute(t *testing.T) {
	def := &Definition{
		ID:    "test",
		Name:  "Test",
		Links: []string{"https://example.com"},
		Search: Search{
			Rows: RowsBlock{
				Selector:                        "items",
				Attribute:                       "downloads",
				Multiple:                        true,
				MissingAttributeEqualsNoResults: true,
			},
			Fields: map[string]Field{
				"title":    {Selector: "..name"},
				"infohash": {Selector: "hash"},
			},
		},
	}

	e := &Engine{id: "test", def: def}

	body := []byte(`{
		"items": [
			{"name": "Movie A"},
			{"name": "Movie B", "downloads": [{"hash": "ABC"}]}
		]
	}`)

	tctx := templateContext{
		Config: map[string]string{},
		Result: map[string]string{},
	}

	results, err := e.extractRowsJSON(body, tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Movie A has no downloads, so should be skipped
	if len(results) != 1 {
		t.Fatalf("expected 1 result (Movie B only), got %d", len(results))
	}
	if results[0].Title != "Movie B" {
		t.Errorf("result[0].Title = %q, want Movie B", results[0].Title)
	}
}
