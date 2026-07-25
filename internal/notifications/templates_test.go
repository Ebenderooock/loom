package notifications

import "testing"

func TestDefaultTemplateFallback(t *testing.T) {
	t.Parallel()
	got := DefaultTemplate(EventType("unknown_event"))
	if got != "{{.EventType}}: {{.Title}}" {
		t.Fatalf("unexpected fallback template: %q", got)
	}
}

func TestRenderTemplate_DefaultAndOverride(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"title":   "Inception",
		"year":    "2010",
		"quality": "1080p",
		"indexer": "IndexerA",
	}

	rendered, err := RenderTemplate("", EventOnGrab, data)
	if err != nil {
		t.Fatalf("RenderTemplate default: %v", err)
	}
	wantDefault := "Grabbed: Inception (2010) — 1080p from IndexerA"
	if rendered != wantDefault {
		t.Fatalf("default rendered = %q, want %q", rendered, wantDefault)
	}

	custom := "{{.Title}}/{{.MediaType}}/{{.EventType}}"
	rendered, err = RenderTemplate(custom, EventOnDownload, map[string]any{
		"title":      "Movie",
		"media_type": "movie",
	})
	if err != nil {
		t.Fatalf("RenderTemplate custom: %v", err)
	}
	if rendered != "Movie/movie/on_download" {
		t.Fatalf("custom rendered = %q", rendered)
	}
}

func TestRenderTemplate_InvalidTemplate(t *testing.T) {
	t.Parallel()
	if _, err := RenderTemplate("{{.Title", EventOnDownload, nil); err == nil {
		t.Fatal("expected parse error for invalid template")
	}
}

func TestAvailableVariables(t *testing.T) {
	t.Parallel()
	vars := AvailableVariables()
	if len(vars) == 0 {
		t.Fatal("expected template variables")
	}

	expected := map[string]bool{
		"{{.Title}}":     false,
		"{{.Year}}":      false,
		"{{.Quality}}":   false,
		"{{.Indexer}}":   false,
		"{{.Size}}":      false,
		"{{.EventType}}": false,
		"{{.MediaType}}": false,
	}
	for _, v := range vars {
		if _, ok := expected[v]; ok {
			expected[v] = true
		}
	}
	for k, seen := range expected {
		if !seen {
			t.Fatalf("missing variable %s in AvailableVariables", k)
		}
	}
}
