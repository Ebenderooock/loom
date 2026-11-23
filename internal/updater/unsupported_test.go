//go:build !linux && !darwin && !windows

package updater

import (
	"strings"
	"testing"
)

func TestUnsupportedNativeInstallFailsClosed(t *testing.T) {
	if supportedNative {
		t.Fatal("unvalidated platform advertised as supported")
	}
	ok, reason := Capability(Binding{Engine: "sqlite"})
	if ok || !strings.Contains(reason, "manual-only") {
		t.Fatalf("unexpected unsupported capability: %v %s", ok, reason)
	}
	for _, args := range [][]string{nil, {"enroll"}, {"recover"}, {"snapshot"}, {"daemon"}, {"install", "C:\\arbitrary.exe"}} {
		if err := Main(t.Context(), args); err == nil {
			t.Fatalf("unsupported operation allowed: %v", args)
		}
	}
	if ok, reason := Capability(Binding{Engine: "postgres"}); ok || !strings.Contains(reason, "PostgreSQL") {
		t.Fatal("missing explicit PostgreSQL manual fallback")
	}
}
