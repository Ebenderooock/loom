package auth

import (
	"strings"
	"testing"
)

func TestGenerateAPIKey(t *testing.T) {
	t.Parallel()
	key, hash, prefix, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.HasPrefix(key, "loom_") {
		t.Fatalf("expected loom_ prefix, got %q", key)
	}
	if len(strings.TrimPrefix(key, "loom_")) != 24 {
		t.Fatalf("unexpected body length: %d", len(key)-5)
	}
	if HashAPIKey(key) != hash {
		t.Fatalf("hash mismatch")
	}
	if got, want := prefix, key[5:13]; got != want {
		t.Fatalf("prefix: want %q got %q", want, got)
	}
}

func TestGenerateAPIKeyUnique(t *testing.T) {
	t.Parallel()
	a, _, _, _ := GenerateAPIKey()
	b, _, _, _ := GenerateAPIKey()
	if a == b {
		t.Fatal("two generated keys collided")
	}
}

func TestParseAPIKey(t *testing.T) {
	t.Parallel()
	good, _, _, _ := GenerateAPIKey()
	if _, err := ParseAPIKey(good); err != nil {
		t.Fatalf("parse good: %v", err)
	}
	for _, bad := range []string{
		"",
		"loom_",
		"loom_short",
		"foo_aaaaaaaaaaaaaaaaaaaaaaaa",
		"loom_!!!!!!!!!!!!!!!!!!!!!!!!",
	} {
		if _, err := ParseAPIKey(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}
