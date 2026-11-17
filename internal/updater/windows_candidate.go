//go:build windows && loom_updater_windows_candidate

package updater

// Native acceptance builds only. The source publishing workflows never set this
// tag. Passing cross-builds is not authorization to enable production shipping.
const windowsCandidate = true
