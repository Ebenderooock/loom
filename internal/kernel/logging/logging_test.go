package logging

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/kernel/config"
)

func TestRedactsSensitiveKeys(t *testing.T) {
	var buf bytes.Buffer
	l, err := newWith(&buf, config.LogConfig{Level: "info", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	l.Info("hello", "api_key", "supersecret", "user", "ada")
	out := buf.String()
	if strings.Contains(out, "supersecret") {
		t.Errorf("api_key value leaked: %s", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Errorf("expected redacted marker; got %s", out)
	}
	if !strings.Contains(out, "ada") {
		t.Errorf("non-sensitive value missing: %s", out)
	}
}

func TestParseLevel(t *testing.T) {
	for _, s := range []string{"debug", "info", "warn", "WARNING", "error"} {
		if _, err := parseLevel(s); err != nil {
			t.Errorf("parseLevel(%q) returned error: %v", s, err)
		}
	}
	if _, err := parseLevel("trace"); err == nil {
		t.Error("expected parseLevel(trace) to error")
	}
}
