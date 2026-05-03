package throttle

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Backoff bounds. The schedule grows 250ms → 500ms → 1s → 2s → 4s and
// is then capped at MaxBackoff regardless of attempt number. Each
// returned value is jittered ±25% to avoid synchronised retries from
// fanning out in lock-step (the "thundering herd" problem).
const (
	BaseBackoff = 250 * time.Millisecond
	MaxBackoff  = 30 * time.Second
)

// Reason classifies why a retry was attempted, surfaced as the
// `reason` label on the loom_indexer_retries_total counter.
type Reason string

const (
	ReasonRateLimited Reason = "rate_limited"  // HTTP 429
	ReasonUnavailable Reason = "unavailable"   // HTTP 503
	ReasonNetwork     Reason = "network_error" // transient transport error
)

// shouldRetry inspects the result of a RoundTrip and reports whether
// the caller should retry. It also surfaces the classification so the
// metrics layer can count by reason. resp may be nil when err is
// non-nil; callers must close any non-nil resp.Body when discarding.
func shouldRetry(resp *http.Response, err error) (Reason, bool) {
	if err != nil {
		// Don't retry caller-initiated cancellation/timeout — that's
		// the operator's deliberate kill switch, not a transient blip.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", false
		}
		// url.Error wraps most transport-level failures; unwrap once
		// so we can look at the underlying classification cleanly.
		var ue *url.Error
		if errors.As(err, &ue) {
			// Cancellation can be wrapped inside *url.Error too.
			if errors.Is(ue.Err, context.Canceled) || errors.Is(ue.Err, context.DeadlineExceeded) {
				return "", false
			}
		}
		// io.ErrUnexpectedEOF, net.OpError on dial/read, TLS
		// handshake timeouts: all retriable.
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return ReasonNetwork, true
		}
		var netErr net.Error
		if errors.As(err, &netErr) {
			if netErr.Timeout() {
				return ReasonNetwork, true
			}
		}
		// Treat any other generic transport error as network-class
		// and retry — matches what curl --retry does.
		return ReasonNetwork, true
	}

	if resp == nil {
		return "", false
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return ReasonRateLimited, true
	case http.StatusServiceUnavailable:
		return ReasonUnavailable, true
	}
	return "", false
}

// parseRetryAfter reads an HTTP `Retry-After` header value. The spec
// allows two forms: a non-negative integer of seconds OR an HTTP-date.
// Returns 0 if the header is empty or unparseable. The reference time
// is supplied so tests can pin "now"; production callers pass
// time.Now().
func parseRetryAfter(header string, now time.Time) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	if secs, err := strconv.Atoi(header); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	// Try HTTP-date forms. http.ParseTime handles RFC1123, RFC850,
	// and ANSI C asctime() — i.e. all three forms RFC 7231 mandates.
	if t, err := http.ParseTime(header); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}

// backoff returns the wait duration for the given (1-based) retry
// attempt. The result is bounded by [50% of base, MaxBackoff] and
// jittered ±25% via the supplied rng. attempt=1 yields the BaseBackoff
// neighbourhood; each subsequent attempt doubles up to the cap.
func backoff(attempt int, rng *rand.Rand) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := BaseBackoff << (attempt - 1)
	if d <= 0 || d > MaxBackoff {
		d = MaxBackoff
	}
	// ±25% jitter. We don't want a strictly-additive jitter because
	// that biases waits longer; symmetric multiplicative is more
	// natural for exponential schedules.
	jitter := 0.75 + rng.Float64()*0.5
	out := time.Duration(float64(d) * jitter)
	if out < BaseBackoff/2 {
		out = BaseBackoff / 2
	}
	if out > MaxBackoff {
		out = MaxBackoff
	}
	return out
}
