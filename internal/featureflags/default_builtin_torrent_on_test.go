//go:build !no_builtin_torrent

package featureflags

import (
	"context"
	"testing"
)

// In a normal build (no no_builtin_torrent tag) the built-in torrent flag must
// default ON so K8s/Docker behaviour is unchanged.
func TestBuiltinTorrentDefaultsOnWithoutTag(t *testing.T) {
	if builtinTorrentDefault != true { //nolint:gosimple // explicit for clarity
		t.Fatalf("builtinTorrentDefault = %v, want true without no_builtin_torrent tag", builtinTorrentDefault)
	}
	d, ok := definition(KeyBuiltinTorrent)
	if !ok {
		t.Fatalf("builtin_torrent flag not registered in Definitions")
	}
	if !d.Default {
		t.Fatalf("builtin_torrent FlagDef.Default = false, want true without tag")
	}

	svc, db := newTestService(t)
	defer db.Close()
	if !svc.Enabled(KeyBuiltinTorrent) {
		t.Fatalf("expected builtin_torrent enabled by default without tag")
	}

	// An explicit override still wins over the build-tag default.
	if err := svc.Set(context.Background(), KeyBuiltinTorrent, false); err != nil {
		t.Fatalf("set: %v", err)
	}
	if svc.Enabled(KeyBuiltinTorrent) {
		t.Fatalf("expected override to disable builtin_torrent")
	}
}
