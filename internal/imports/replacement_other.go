//go:build !darwin

package imports

import "os"

func publishReplacement(staged, dst string) error {
	return os.Rename(staged, dst)
}
