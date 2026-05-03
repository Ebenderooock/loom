package throttle

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Options are the optional knobs Wrap accepts. Zero-value is fine for
// production; tests pin Now/Rand for determinism.
type Options struct {
	Now    func() time.Time
	Rand   *rand.Rand
	Logger *slog.Logger
}

// Wrap returns a RoundTripper that:
//
//  1. Acquires a token from the per-indexer bucket before every
//     attempt (steady-state rate limiting).
//  2. Honours Retry-After on 429 / 503 responses verbatim.
//  3. Otherwise applies exponential backoff with jitter to retriable
//     failures, up to cfg.MaxRetries.
//  4. Surfaces metrics under loom_indexer_*.
//
// indexerID labels metrics and log lines; kind ("newznab", "cardigann")
// is used as a coarse partition for dashboards. base is the upstream
// transport (proxy + auth layers already composed).
func Wrap(base http.RoundTripper, indexerID, kind string, cfg Config, opts Options) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	cfg = Resolve(cfg)
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	rng := opts.Rand
	if rng == nil {
		// Per-transport rand source so concurrent transports don't
		// contend on the global mutex.
		rng = rand.New(rand.NewSource(now().UnixNano()))
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &transport{
		base:      base,
		bucket:    newBucketWithClock(cfg.PerMinute, cfg.Burst, now),
		cfg:       cfg,
		indexerID: indexerID,
		kind:      kind,
		now:       now,
		rng:       rng,
		rngMu:     &sync.Mutex{},
		logger:    logger.With(slog.String("indexer", indexerID), slog.String("component", "indexers.throttle")),
	}
}

type transport struct {
	base      http.RoundTripper
	bucket    *Bucket
	cfg       Config
	indexerID string
	kind      string

	now   func() time.Time
	rng   *rand.Rand
	rngMu *sync.Mutex

	logger *slog.Logger
}

// RoundTrip implements http.RoundTripper.
func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := t.now()
	body, err := readAndRewind(req)
	if err != nil {
		observeRequest(t.indexerID, t.kind, OutcomeError, t.now().Sub(start).Seconds())
		return nil, err
	}

	ctx := req.Context()
	maxAttempts := t.cfg.MaxRetries + 1

	var (
		resp     *http.Response
		lastErr  error
		attempts int
	)

	for attempts = 0; attempts < maxAttempts; attempts++ {
		// Bucket — block until a token is available or ctx cancels.
		waited, werr := t.bucket.Acquire(ctx)
		observeRateLimitWait(t.indexerID, waited.Seconds())
		if werr != nil {
			observeRequest(t.indexerID, t.kind, OutcomeError, t.now().Sub(start).Seconds())
			return nil, werr
		}

		// Replay the body for every attempt — once http.Transport has
		// consumed it, it's gone, so we have to hand it a fresh reader.
		if body != nil {
			req.Body = io.NopCloser(bytes.NewReader(body))
			req.ContentLength = int64(len(body))
			req.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(body)), nil
			}
		}

		resp, lastErr = t.base.RoundTrip(req)
		reason, retry := shouldRetry(resp, lastErr)
		if !retry {
			return finishOutcome(t, start, resp, lastErr)
		}
		// Out of attempts? Surface the last response/error to the
		// caller verbatim so they see the real failure (and any
		// Retry-After hint) rather than an opaque nil.
		if attempts+1 >= maxAttempts {
			observeRetry(t.indexerID, reason)
			return finishOutcome(t, start, resp, lastErr)
		}
		// Retriable and we have attempts left. Drain + close so the
		// connection can be reused before we sleep.
		drainBody(resp)

		// Compute wait: Retry-After wins when present, otherwise the
		// exponential schedule.
		wait := t.computeWait(attempts+1, reason, lastErr, resp)
		resp = nil
		observeRetry(t.indexerID, reason)
		t.logger.Debug("retrying indexer request",
			slog.String("kind", t.kind),
			slog.String("reason", string(reason)),
			slog.Int("attempt", attempts+1),
			slog.Duration("wait", wait),
		)

		// Sleep, but bail out fast on ctx cancellation.
		if wait > 0 {
			tmr := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				tmr.Stop()
				observeRequest(t.indexerID, t.kind, OutcomeError, t.now().Sub(start).Seconds())
				return nil, ctx.Err()
			case <-tmr.C:
			}
		}
	}

	// Defensive: should be unreachable given the in-loop returns.
	return finishOutcome(t, start, resp, lastErr)
}

// computeWait returns how long to sleep before the next attempt.
// For 429/503 we honour Retry-After exactly (capped at MaxBackoff to
// avoid a misbehaving server stalling us forever); otherwise we
// schedule an exponential backoff with jitter.
func (t *transport) computeWait(nextAttempt int, reason Reason, _ error, resp *http.Response) time.Duration {
	if resp != nil && (reason == ReasonRateLimited || reason == ReasonUnavailable) {
		if v := resp.Header.Get("Retry-After"); v != "" {
			if d := parseRetryAfter(v, t.now()); d > 0 {
				if d > MaxBackoff {
					d = MaxBackoff
				}
				return d
			}
		}
	}
	t.rngMu.Lock()
	defer t.rngMu.Unlock()
	return backoff(nextAttempt, t.rng)
}

// finishOutcome records the final metric label and returns to caller.
func finishOutcome(t *transport, start time.Time, resp *http.Response, err error) (*http.Response, error) {
	dur := t.now().Sub(start).Seconds()
	if err != nil {
		observeRequest(t.indexerID, t.kind, OutcomeError, dur)
		return nil, err
	}
	if resp == nil {
		// Shouldn't happen — both resp and err nil means RoundTrip
		// produced no result. Surface as a generic transport error
		// rather than panicking on resp.StatusCode below.
		observeRequest(t.indexerID, t.kind, OutcomeError, dur)
		return nil, errors.New("indexers/throttle: nil response with nil error")
	}
	outcome := OutcomeSuccess
	switch {
	case resp.StatusCode >= 500:
		outcome = OutcomeServerError
	case resp.StatusCode >= 400:
		outcome = OutcomeClientError
	}
	observeRequest(t.indexerID, t.kind, outcome, dur)
	return resp, nil
}

// readAndRewind buffers the request body so the retry loop can replay
// it on every attempt. Returns nil for GET-style requests with no body.
func readAndRewind(req *http.Request) ([]byte, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	// Honour an explicit GetBody if the caller already provided one —
	// that's the contract http.Request defines for safe replay.
	if req.GetBody != nil {
		// Pull a fresh reader from GetBody and buffer that, so the
		// original body remains untouched.
		rc, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil, err
	}
	return body, nil
}

// drainBody consumes and closes a response body so the underlying
// connection can be reused. We cap the drain at 4 KiB to bound the
// time we spend on a misbehaving server's giant 503 page.
func drainBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
}

// Sentinel returned by callers that build a malformed request. Kept
// public so the indexer kinds can identify it without depending on
// http internals.
var ErrMalformedRequest = errors.New("indexers/throttle: malformed request")

// helper used by tests to keep the linter quiet about unused imports.
var _ = strconv.Itoa
var _ context.Context
