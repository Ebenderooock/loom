package updater

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ebenderooock/loom/internal/buildinfo"
	"github.com/ebenderooock/loom/internal/updates"
)

// These paths are protocol constants, never supplied by an HTTP client, queued
// request, environment variable, manifest, or service-owned configuration.
const (
	RuntimePath = "/api/v1/updates/native-runtime"
)

// Windows uses OS known folders, not caller/environment-provided destinations.
var (
	tokenPath  = filepath.Join(BinDir, "updater-token")
	markerPath = filepath.Join(BinDir, "updater-managed")
)

// Binding contains the service's actual effective configuration, not defaults.
type Binding struct {
	ConfigDir, DataDir, SQLitePath, Engine, Listen string
}

func (b Binding) fixed() bool {
	return b.ConfigDir == ConfigDir && b.DataDir == DataDir && b.SQLitePath == DBPath &&
		b.Engine == "sqlite" && b.Listen == "127.0.0.1:1925"
}

// Capability is used only for consent/UI gating. The root helper independently
// validates the loaded unit, paths, files, state and consent before every apply.
func Capability(b Binding) (bool, string) {
	if b.Engine == "postgres" {
		return false, "PostgreSQL installations are manual-update only; signed notices remain available."
	}
	if !supportedNative {
		return false, "This platform is manual-only; no validated native updater is shipped. Signed update notices remain available."
	}
	if !b.fixed() {
		return false, "Automatic installation requires an installer-managed " + nativeManager + " service with fixed SQLite, config and loopback paths. Containers and Kubernetes use image/orchestration updates."
	}
	if err := nativeInstalled(); err != nil {
		return false, err.Error()
	}
	if err := enrollmentStatus(); err != nil {
		return false, err.Error()
	}
	return true, nativeManager + " SQLite updates use the privileged helper, stopped-service snapshots and rollback. Updates restart Loom. Unsupported or unmanaged installations remain manual-only."
}

// RuntimeHandler exposes only version/readiness/consent to the installed helper.
// Its separate random token is never a session, API key, or generic admin bypass.
func RuntimeHandler(b Binding, settings func(context.Context) (updates.Settings, error), ready func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !b.fixed() || nativeInstalled() != nil {
			http.NotFound(w, r)
			return
		}
		token, err := os.ReadFile(tokenPath) // #nosec G304 -- fixed protected installer token, not request data
		given := r.Header.Get("X-Loom-Updater-Token")
		if err != nil || len(token) != 64 || len(given) != 64 || subtle.ConstantTimeCompare(token, []byte(given)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		s, err := settings(r.Context())
		if err != nil {
			http.Error(w, "settings unavailable", http.StatusServiceUnavailable)
			return
		}
		ref := buildinfo.SourceRef
		channel := "stable"
		if len(ref) >= 11 && ref[:11] == "refs/heads/" {
			channel = "prelease"
		}
		if ref == "unknown" {
			ref = "refs/tags/v" + trimV(buildinfo.Version)
		}
		value := Runtime{
			Identity: Identity{Version: buildinfo.Version, SHA: buildinfo.Commit, Ref: ref, Channel: channel},
			Native:   true, Ready: ready(r.Context()) == nil, Engine: b.Engine, Settings: s,
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(value)
	}
}

func trimV(s string) string {
	if len(s) > 0 && s[0] == 'v' {
		return s[1:]
	}
	return s
}
