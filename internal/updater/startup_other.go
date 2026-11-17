//go:build !darwin && !windows

package updater

import "context"

// StartupGuard is a no-op where service-manager ordering supplies the barrier
// (Linux), or privileged installation is unsupported.
func StartupGuard(context.Context) error { return nil }
