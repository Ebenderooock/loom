//go:build !windows

package libraries

import (
	"os"
	"syscall"
)

func healthLinkCount(info os.FileInfo) uint64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(stat.Nlink)
	}
	return 0
}
