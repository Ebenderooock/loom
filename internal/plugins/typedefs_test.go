package plugins

import (
	"strings"
	"testing"
)

// TestTypeDefsCoverAllEvents guards against the editor type defs drifting from
// the supported event list: every event key must appear exactly once as a
// discriminated-union literal `event: "<key>"`.
func TestTypeDefsCoverAllEvents(t *testing.T) {
	for _, ev := range SupportedEvents {
		needle := `event: "` + ev.Key + `"`
		if n := strings.Count(PluginTypeDefs, needle); n != 1 {
			t.Errorf("event %q: expected exactly one union member (%s), found %d", ev.Key, needle, n)
		}
	}
}

// TestTypeDefsNoUnknownEvents ensures the type defs don't declare union members
// for events that aren't actually supported.
func TestTypeDefsNoUnknownEvents(t *testing.T) {
	// Each ` event: "..."` occurrence must correspond to a supported key.
	const marker = `  | (LoomEventBase & { event: "`
	for _, line := range strings.Split(PluginTypeDefs, "\n") {
		if !strings.HasPrefix(line, marker) {
			continue
		}
		rest := strings.TrimPrefix(line, marker)
		key := rest[:strings.IndexByte(rest, '"')]
		if _, ok := eventByKey(key); !ok {
			t.Errorf("type defs declare unknown event key %q", key)
		}
	}
}

func TestTypeDefsDeclareGlobals(t *testing.T) {
	for _, want := range []string{
		"declare const event: LoomEvent;",
		"declare const env:",
		"declare const console:",
		"declare function fetch(url: string",
	} {
		if !strings.Contains(PluginTypeDefs, want) {
			t.Errorf("type defs missing global declaration: %q", want)
		}
	}
}
