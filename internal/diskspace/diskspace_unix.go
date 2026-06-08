//go:build !windows

package diskspace

import "golang.org/x/sys/unix"

func get(path string) (total, free uint64, err error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	// The integer field types of unix.Statfs_t vary across platforms (e.g.
	// Bavail is int64 on the BSDs but uint64 on Linux), so normalize them.
	bsize := toUint64(stat.Bsize)
	total = toUint64(stat.Blocks) * bsize
	free = toUint64(stat.Bavail) * bsize
	return total, free, nil
}

// toUint64 widens the platform-dependent signed/unsigned block-count fields of
// unix.Statfs_t to uint64. Block counts and sizes are non-negative, so the
// conversion is always safe.
func toUint64[T int32 | int64 | uint32 | uint64](v T) uint64 {
	return uint64(v) //nolint:gosec // filesystem block counts are non-negative
}
