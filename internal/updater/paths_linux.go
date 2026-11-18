//go:build linux

package updater

// Fixed installer-managed Linux paths, never taken from service input.
const (
	BinDir          = "/usr/local/lib/loom"
	ConfigDir       = "/etc/loom"
	DataDir         = "/var/lib/loom"
	DBPath          = DataDir + "/loom.db"
	StateDir        = "/var/lib/loom-updater"
	supportedNative = true
	nativeManager   = "Linux/systemd"
)
