package buildinfo

import (
	"strings"
	"testing"
)

func TestStringIncludesVersionAndRuntime(t *testing.T) {
	got := String()
	for _, want := range []string{Version, Commit, Date} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, missing %q", got, want)
		}
	}
}
