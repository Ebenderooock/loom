//go:build linux || darwin

package updater

import (
	"errors"
	"os"
	"syscall"
)

func metadata(path string, info os.FileInfo) (fileMeta, uint64, error) {
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileMeta{}, 0, errors.New("file ownership unavailable")
	}
	return fileMeta{Path: path, Mode: uint32(info.Mode().Perm()), UID: int(s.Uid), GID: int(s.Gid), Dir: info.IsDir()}, uint64(s.Nlink), nil //nolint:unconvert // Nlink is uint16 on Darwin but uint64 on Linux.
}
