package torrent

import (
	"testing"

	"golang.org/x/time/rate"
)

func TestNewRateLimiterUnlimited(t *testing.T) {
	for _, v := range []int64{0, -1, -1024} {
		l := newRateLimiter(v)
		if l.Limit() != rate.Inf {
			t.Fatalf("newRateLimiter(%d): expected Inf limit, got %v", v, l.Limit())
		}
	}
}

func TestNewRateLimiterFinite(t *testing.T) {
	// Below the minimum burst: burst is clamped up to rateLimiterBurst.
	l := newRateLimiter(1000)
	if l.Limit() != rate.Limit(1000) {
		t.Fatalf("expected limit 1000, got %v", l.Limit())
	}
	if l.Burst() != rateLimiterBurst {
		t.Fatalf("expected burst %d, got %d", rateLimiterBurst, l.Burst())
	}

	// Above the minimum burst: burst equals the rate.
	big := int64(4 << 20)
	l = newRateLimiter(big)
	if l.Burst() != int(big) {
		t.Fatalf("expected burst %d, got %d", big, l.Burst())
	}
}

func TestApplyRateLimit(t *testing.T) {
	l := newRateLimiter(0) // start unlimited

	applyRateLimit(l, 2000)
	if l.Limit() != rate.Limit(2000) {
		t.Fatalf("expected limit 2000, got %v", l.Limit())
	}
	if l.Burst() != rateLimiterBurst {
		t.Fatalf("expected burst %d, got %d", rateLimiterBurst, l.Burst())
	}

	// Back to unlimited.
	applyRateLimit(l, 0)
	if l.Limit() != rate.Inf {
		t.Fatalf("expected Inf limit after reset, got %v", l.Limit())
	}

	// Nil limiter must not panic.
	applyRateLimit(nil, 1000)
}
