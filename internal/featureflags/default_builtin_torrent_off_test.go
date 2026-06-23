//go:build no_builtin_torrent

package featureflags

import "testing"

// When built with -tags no_builtin_torrent the built-in torrent flag must
// default OFF so single-binary distros (e.g. Synology) never offer it.
func TestBuiltinTorrentDefaultsOffWithTag(t *testing.T) {
	if builtinTorrentDefault != false { //nolint:gosimple // explicit for clarity
		t.Fatalf("builtinTorrentDefault = %v, want false with no_builtin_torrent tag", builtinTorrentDefault)
	}
	d, ok := definition(KeyBuiltinTorrent)
	if !ok {
		t.Fatalf("builtin_torrent flag not registered in Definitions")
	}
	if d.Default {
		t.Fatalf("builtin_torrent FlagDef.Default = true, want false with tag")
	}

	svc, db := newTestService(t)
	defer db.Close()
	if svc.Enabled(KeyBuiltinTorrent) {
		t.Fatalf("expected builtin_torrent disabled by default with tag")
	}
}
