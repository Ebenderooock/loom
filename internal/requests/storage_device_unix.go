//go:build !windows

package requests

import (
	"os"
	"syscall"
)

func sameStorageDevice(_, _ string, root, file os.FileInfo) bool {
	rootStat, rootOK := root.Sys().(*syscall.Stat_t)
	fileStat, fileOK := file.Sys().(*syscall.Stat_t)
	return rootOK && fileOK && rootStat.Dev == fileStat.Dev
}
