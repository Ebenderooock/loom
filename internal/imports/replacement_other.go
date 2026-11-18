//go:build !darwin

package imports

import "os"

const replacementSwapsFiles = false

func publishReplacement(staged, dst string) error {
	return os.Rename(staged, dst)
}
