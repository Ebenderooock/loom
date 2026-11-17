//go:build !linux && !darwin && !windows

package updater

// No paths are enrolled on unsupported platforms.
const (
	BinDir          = ""
	ConfigDir       = ""
	DataDir         = ""
	DBPath          = ""
	StateDir        = ""
	supportedNative = false
	nativeManager   = "unsupported native"
)
