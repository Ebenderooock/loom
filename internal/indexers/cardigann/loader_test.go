package cardigann

import (
	"os"
	"path/filepath"
	"testing"
)

// validYAML is the smallest definition that satisfies validate(): a
// site name, one link, a search block with rows.selector and at least
// one field. We re-use this fixture in tests that need a known-good
// starting point and mutate copies for the negative cases.
const validYAML = `---
site: example
name: Example
type: public
language: en
encoding: UTF-8
links:
  - https://example.test
caps:
  modes:
    search: ["q"]
search:
  paths:
    - path: search.php
  rows:
    selector: "tr.row"
  fields:
    title:
      selector: "a.title"
    download:
      selector: "a.dl"
      attribute: href
`

func TestLoadFile_ValidDefinition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.yml")
	if err := os.WriteFile(path, []byte(validYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	def, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	if def.Site != "example" || len(def.Links) != 1 {
		t.Errorf("unexpected definition: %+v", def)
	}
}

func TestLoadFile_ParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yml")
	// Unbalanced bracket — yaml.v3 reports a syntax error.
	if err := os.WriteFile(path, []byte("site: [unterminated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestParseDefinition_MissingFields(t *testing.T) {
	cases := map[string]string{
		"no site":     `links: ["https://x"]`,
		"no links":    `site: x`,
		"no rows sel": `site: x
links: ["https://x"]
search:
  paths: [{path: s}]
  fields: {title: {selector: a}}`,
		"no fields": `site: x
links: ["https://x"]
search:
  paths: [{path: s}]
  rows: {selector: tr}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDefinition([]byte(body)); err == nil {
				t.Fatalf("expected validation error for %q", name)
			}
		})
	}
}

func TestLoader_ReloadAndGet(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "example.yml"), []byte(validYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.yml"), []byte("site: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	l := NewLoader(dir)
	defs, errs := l.Reload()
	if len(defs) != 1 {
		t.Fatalf("expected 1 valid definition, got %d", len(defs))
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 load error, got %d", len(errs))
	}
	if _, ok := l.Get("example"); !ok {
		t.Errorf("Get(example) returned !ok")
	}
	if _, ok := l.Get("missing"); ok {
		t.Errorf("Get(missing) returned ok")
	}
}

func TestLoader_MissingDir(t *testing.T) {
	l := NewLoader(filepath.Join(t.TempDir(), "does-not-exist"))
	defs, errs := l.Reload()
	if len(defs) != 0 || len(errs) != 0 {
		t.Errorf("expected silent no-op for missing dir, got defs=%d errs=%d", len(defs), len(errs))
	}
}
