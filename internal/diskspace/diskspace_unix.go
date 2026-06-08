//go:build !windows

package diskspace

import "golang.org/x/sys/unix"

func get(path string) (total, free uint64, err error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	// Field types vary across Unix platforms (e.g. Bavail is int64 on the
	// BSDs, uint64 on Linux), so cast everything to uint64.
	bsize := uint64(stat.Bsize)
	total = uint64(stat.Blocks) * bsize
	free = uint64(stat.Bavail) * bsize
	return total, free, nil
}
