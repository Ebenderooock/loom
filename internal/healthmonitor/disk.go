package healthmonitor

import "syscall"

// checkDiskSpace returns total and free disk space in GB for the given path.
func checkDiskSpace(path string) (totalGB, freeGB float64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	return float64(total) / (1 << 30), float64(free) / (1 << 30), nil
}
