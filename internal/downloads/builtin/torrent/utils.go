package torrent

import (
	"golang.org/x/sys/unix"
	"golang.org/x/time/rate"
)

// diskFreeBytes returns the number of available bytes on the filesystem
// that contains path.
func diskFreeBytes(path string) (int64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return -1, err
	}
	// Bavail is blocks available to unprivileged processes.
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}

// unlimitedRateLimiter returns a rate.Limiter that imposes no cap.
func unlimitedRateLimiter() *rate.Limiter {
	return rate.NewLimiter(rate.Inf, 0)
}

// applyLimit sets a rate.Limiter to the given bytes-per-second cap.
// Burst is set to 2× the rate so short bursts aren't throttled too
// aggressively while still respecting the average limit.
func applyLimit(l *rate.Limiter, bytesPerSec int64) {
	r := rate.Limit(bytesPerSec)
	burst := int(bytesPerSec * 2)
	if burst < 1 {
		burst = 1
	}
	l.SetLimit(r)
	l.SetBurst(burst)
}
