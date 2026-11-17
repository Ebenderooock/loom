//go:build darwin

package updater

// Fixed installer-managed macOS paths, never taken from service input.
const (
	macBase         = "/Library/Application Support/Loom"
	BinDir          = macBase + "/bin"
	ConfigDir       = macBase + "/config"
	DataDir         = macBase + "/data"
	DBPath          = DataDir + "/loom.db"
	StateDir        = macBase + "/updater"
	supportedNative = true
	nativeManager   = "macOS/launchd"
	macService      = "system/dev.loom.server"
	macPlist        = "/Library/LaunchDaemons/dev.loom.server.plist"
	macUpdaterPlist = "/Library/LaunchDaemons/dev.loom.updater.plist"
	macSocket       = BinDir + "/updater.sock"
)
