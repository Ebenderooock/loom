package healthmonitor

import "github.com/ebenderooock/loom/internal/diskspace"

// checkDiskSpace returns total and free disk space in GB for the given path.
func checkDiskSpace(path string) (totalGB, freeGB float64, err error) {
	total, free, err := diskspace.Get(path)
	if err != nil {
		return 0, 0, err
	}
	return float64(total) / (1 << 30), float64(free) / (1 << 30), nil
}
