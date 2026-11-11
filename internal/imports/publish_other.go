//go:build !linux && !darwin && !windows

package imports

import "os"

func publishCopy(staged, dst string) error {
	return os.Link(staged, dst)
}
