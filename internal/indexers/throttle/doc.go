// Package throttle provides per-indexer rate limiting and retry/backoff
// for the outbound HTTP transports used by indexer kinds (Newznab,
// Cardigann, ...). It composes around a base http.RoundTripper so the
// existing proxy/auth wiring is preserved untouched:
//
//	base → proxy → ratelimit → retry
//
// The token bucket bounds the *steady-state* request rate and absorbs
// short bursts; the retry loop layers on top, honouring 429/503
// Retry-After headers and using exponential backoff with jitter for
// other transient failures.
//
// All knobs are per-indexer and persisted on the indexers table; the
// package falls back to safe defaults (60 req/min, burst 5, 3 retries)
// when the row leaves them NULL. Metrics register lazily against the
// telemetry package's prometheus registry, so simply importing this
// package wires the dashboard up.
package throttle
