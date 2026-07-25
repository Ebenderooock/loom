package validation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSizedFile(t *testing.T, path string, size int) {
	t.Helper()
	data := make([]byte, size)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func TestDefaultRulesAndConstructor(t *testing.T) {
	t.Parallel()
	v := NewValidator(nil)
	rules := v.Rules()
	if len(rules) == 0 {
		t.Fatal("expected default rules when constructor receives nil")
	}
}

func TestValidatorFileSizeRule(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	tests := []struct {
		name     string
		fileName string
		size     int
		ruleCfg  map[string]any
		pass     bool
	}{
		{
			name:     "zero bytes fails",
			fileName: "movie.1080p.mkv",
			size:     0,
			ruleCfg:  map[string]any{"min_bytes_1080p": 1},
			pass:     false,
		},
		{
			name:     "below quality threshold fails",
			fileName: "movie.1080p.mkv",
			size:     100,
			ruleCfg:  map[string]any{"min_bytes_1080p": 200},
			pass:     false,
		},
		{
			name:     "above quality threshold passes",
			fileName: "movie.1080p.mkv",
			size:     300,
			ruleCfg:  map[string]any{"min_bytes_1080p": 200},
			pass:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.fileName)
			writeSizedFile(t, path, tt.size)

			v := NewValidator([]ValidationRule{{
				ID:      "file_size",
				Enabled: true,
				Config:  tt.ruleCfg,
			}})
			res := v.Validate(path)
			if res.Valid != tt.pass {
				t.Fatalf("Valid = %v, want %v (checks=%+v)", res.Valid, tt.pass, res.Checks)
			}
		})
	}
}

func TestValidatorExtensionRule(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	v := NewValidator([]ValidationRule{{ID: "extension", Enabled: true}})

	tests := []struct {
		name     string
		fileName string
		pass     bool
		msgPart  string
	}{
		{"video extension passes", "movie.mkv", true, "valid"},
		{"subtitle extension passes", "movie.srt", true, "subtitle/info"},
		{"dangerous extension fails", "movie.exe", false, "dangerous"},
		{"unknown extension fails", "movie.abc", false, "unrecognized"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.fileName)
			writeSizedFile(t, path, 16)
			res := v.Validate(path)
			if res.Valid != tt.pass {
				t.Fatalf("Valid = %v, want %v", res.Valid, tt.pass)
			}
			if len(res.Checks) != 1 || !strings.Contains(res.Checks[0].Message, tt.msgPart) {
				t.Fatalf("unexpected check message: %+v", res.Checks)
			}
		})
	}
}

func TestValidatorArchiveRule(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	v := NewValidator([]ValidationRule{{ID: "archive_detection", Enabled: true}})

	zipFile := filepath.Join(dir, "movie.zip")
	writeSizedFile(t, zipFile, 32)
	res := v.Validate(zipFile)
	if res.Valid {
		t.Fatalf("expected archive file to be rejected: %+v", res)
	}

	mkvFile := filepath.Join(dir, "movie.mkv")
	writeSizedFile(t, mkvFile, 32)
	res = v.Validate(mkvFile)
	if !res.Valid {
		t.Fatalf("expected non-archive to pass: %+v", res)
	}
}

func TestValidatorMiscRules(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "movie.mkv")
	writeSizedFile(t, path, 128)

	v := NewValidator([]ValidationRule{
		{ID: "min_duration", Enabled: true},
		{ID: "unknown_rule", Enabled: true},
		{ID: "file_size", Enabled: false},
	})
	res := v.Validate(path)
	if !res.Valid {
		t.Fatalf("expected checks to pass: %+v", res)
	}
	if len(res.Checks) != 2 {
		t.Fatalf("expected 2 enabled checks, got %d", len(res.Checks))
	}
}
