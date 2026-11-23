//go:build windows

package updater

import (
	"errors"
	"path/filepath"

	"golang.org/x/sys/windows"
)

const (
	winServiceSID    = "S-1-5-80-1798712599-1982871309-4130942312-256610295-2568066399"
	winAccount       = `NT SERVICE\Loom`
	winHelperService = "LoomUpdater"
	winMarker        = "loom-scm-updater-v1\n"
	winGateName      = `Global\Loom.Updater.Start.v1`
	winJobName       = `Global\Loom.Updater.Processes.v1`
	nativeManager    = "Windows/SCM"
	// Deliberate activation barrier: cross-compilation is not native validation.
	// Only the explicit candidate build used by native acceptance CI enables it.
	supportedNative = windowsCandidate
)

// Fixed OS-known-folder layout. Never fall back to ProgramFiles/ProgramData env.
//
//nolint:revive // Resolve the complete OS-known-folder tuple once, before dependent path initialization.
var (
	BinDir, ConfigDir, DataDir, DBPath, StateDir, winPathErr = winKnownPaths()
)

func winKnownPaths() (string, string, string, string, string, error) {
	pf, e1 := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	pd, e2 := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if e1 != nil || e2 != nil {
		return "", "", "", "", "", errors.New("native Windows known folders unavailable")
	}
	base := filepath.Join(pd, "Loom")
	data := filepath.Join(base, "data")
	return filepath.Join(pf, "Loom"), filepath.Join(base, "config"), data, filepath.Join(data, "loom.db"), filepath.Join(base, "updater"), nil
}
