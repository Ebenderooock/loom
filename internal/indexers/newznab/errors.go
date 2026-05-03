package newznab

import "errors"

// Typed errors surfaced by the client. They wrap the underlying cause
// (network, XML, HTTP status) so callers can branch with errors.Is
// and the test suite can assert classification without matching free
// text.
var (
	// ErrAuthFailed maps to upstream `<error code="100">` and to HTTP
	// 401/403 — the api_key or auth header is wrong.
	ErrAuthFailed = errors.New("newznab: auth failed")

	// ErrCapsParse signals the caps document came back but did not
	// match the Newznab schema. Treat as a configuration error.
	ErrCapsParse = errors.New("newznab: caps parse failed")

	// ErrRateLimited maps to HTTP 429. The HealthChecker downgrades
	// the indexer to "degraded" rather than "failed" on this.
	ErrRateLimited = errors.New("newznab: rate limited")

	// ErrUpstream covers any 5xx from the upstream that isn't
	// otherwise classified.
	ErrUpstream = errors.New("newznab: upstream error")

	// ErrTimeout wraps context.DeadlineExceeded on the outbound HTTP
	// call.
	ErrTimeout = errors.New("newznab: timeout")

	// ErrMalformedXML is returned when the body is not parseable as
	// XML at all (e.g. an HTML error page returned where XML was
	// expected).
	ErrMalformedXML = errors.New("newznab: malformed xml")
)
