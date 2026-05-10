package newznab

import (
	"errors"
	"fmt"

	"github.com/ebenderooock/loom/internal/indexers"
)

// Typed errors surfaced by the client. They wrap the underlying cause
// (network, XML, HTTP status) so callers can branch with errors.Is
// and the test suite can assert classification without matching free
// text.
//
// ErrRateLimited and ErrTimeout also wrap the package-level
// indexers.ErrIndexerRateLimited / ErrIndexerTimeout sentinels so the
// service layer can classify faults uniformly across all indexer kinds.
var (
	// ErrAuthFailed maps to upstream `<error code="100">` and to HTTP
	// 401/403 — the api_key or auth header is wrong.
	ErrAuthFailed = errors.New("newznab: auth failed")

	// ErrCapsParse signals the caps document came back but did not
	// match the Newznab schema. Treat as a configuration error.
	ErrCapsParse = errors.New("newznab: caps parse failed")

	// ErrRateLimited maps to HTTP 429. The service marks the indexer
	// as "degraded" rather than "failed" on this.
	ErrRateLimited = fmt.Errorf("newznab: rate limited: %w", indexers.ErrIndexerRateLimited)

	// ErrUpstream covers any 5xx from the upstream that isn't
	// otherwise classified.
	ErrUpstream = errors.New("newznab: upstream error")

	// ErrTimeout wraps context.DeadlineExceeded on the outbound HTTP
	// call. Also wraps the package-level timeout sentinel.
	ErrTimeout = fmt.Errorf("newznab: timeout: %w", indexers.ErrIndexerTimeout)

	// ErrMalformedXML is returned when the body is not parseable as
	// XML at all (e.g. an HTML error page returned where XML was
	// expected).
	ErrMalformedXML = errors.New("newznab: malformed xml")

	// ErrCloudFlare is returned when the response body looks like a
	// Cloudflare challenge page (JS challenge or CAPTCHA). The
	// upstream is reachable but a bot-detection layer is blocking.
	ErrCloudFlare = errors.New("newznab: cloudflare challenge")
)
