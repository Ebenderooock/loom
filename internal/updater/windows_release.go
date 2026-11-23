//go:build windows && !loom_updater_windows_candidate

package updater

// Production enrollment remains blocked until native acceptance has
// verified the candidate on NTFS/SCM, including power/restart failure recovery.
const windowsCandidate = false
