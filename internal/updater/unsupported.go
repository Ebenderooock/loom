//go:build !linux && !darwin && !windows

package updater

import (
	"context"
	"errors"
	"os"
)

func nativeInstalled() error {
	return errors.New("this platform remains manual-only: a validated SCM recovery/startup barrier, NTFS security-descriptor-preserving transaction and LocalService process containment are not implemented; no privileged Windows updater is shipped")
}
func enrollmentStatus() error { return nativeInstalled() }
func noACL(_ *os.File) error  { return nil } // transaction tests only; no privileged entry point

// Main fails closed on platforms without a privileged helper implementation.
func Main(_ context.Context, _ []string) error { return nativeInstalled() }
