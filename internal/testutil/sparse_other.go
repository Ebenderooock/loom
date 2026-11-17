//go:build !windows

package testutil

import "os"

// MarkSparse is unnecessary on platforms where extending a file leaves holes.
func MarkSparse(_ *os.File) error {
	return nil
}
