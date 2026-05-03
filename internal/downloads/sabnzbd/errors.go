package sabnzbd

import "errors"

// Error sentinels returned by the SABnzbd kind. Callers compare with
// errors.Is; the wrapped form (returned by package methods) carries
// upstream context — the SAB error string, the failing nzo_id, etc.
var (
	// ErrAuth indicates SABnzbd rejected the API key. Most often
	// surfaces as a 200 response with {"status":false,"error":"..."}
	// rather than HTTP 401, because SAB validates apikey in
	// application code.
	ErrAuth = errors.New("sabnzbd: API key rejected")

	// ErrUpstream is a transport- or server-side failure: connection
	// refused, 5xx, malformed JSON envelope.
	ErrUpstream = errors.New("sabnzbd: upstream request failed")

	// ErrNotFound is returned when a queue/history lookup by nzo_id
	// has no matching item. Distinct from ErrAuth so callers can
	// retry or surface a 404.
	ErrNotFound = errors.New("sabnzbd: nzo_id not found")

	// ErrServer is the SAB-reported error envelope:
	// {"status":false,"error":"..."}. The wrapped error includes the
	// upstream message verbatim.
	ErrServer = errors.New("sabnzbd: server reported error")

	// ErrMalformedNZB is surfaced when the user-supplied AddRequest
	// is missing both NZBURL and RawBytes, or when SAB rejects the
	// upload as unparseable.
	ErrMalformedNZB = errors.New("sabnzbd: NZB payload is missing or malformed")

	// ErrConfig is returned by the factory when the persisted
	// Definition.Config blob cannot be parsed.
	ErrConfig = errors.New("sabnzbd: invalid client config")
)
