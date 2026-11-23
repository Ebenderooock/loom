//go:build !linux && !darwin && !windows

package updater

import (
	"errors"
	"os"
)

func metadata(_ string, _ os.FileInfo) (fileMeta, uint64, error) {
	return fileMeta{}, 0, errors.New("native updater ownership validation unsupported")
}
