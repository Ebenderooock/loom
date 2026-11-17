//go:build !windows

package imports

import "os"

func readReplacementDestination(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func replacementBlockedByOpenReader(_ error, _, _ string) bool {
	return false
}
