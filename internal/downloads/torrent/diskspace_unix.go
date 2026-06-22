//go:build !windows

package torrent

import "syscall"

func diskFreeSpace(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return -1, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
