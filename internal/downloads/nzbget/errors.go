package nzbget

import "errors"

// Error sentinels returned by the NZBGet kind. Callers compare with
// errors.Is; the wrapped form (returned by package methods) carries
// upstream context — the JSON-RPC error message, the failing NZBID,
// the HTTP status, etc.
var (
	// ErrAuth indicates NZBGet rejected the ControlUsername /
	// ControlPassword pair. NZBGet returns HTTP 401 when Basic Auth
	// fails before the JSON-RPC envelope is even consulted, so this
	// surfaces from the transport layer.
	ErrAuth = errors.New("nzbget: control credentials rejected")

	// ErrUpstream is a transport- or server-side failure: connection
	// refused, 5xx, malformed JSON envelope, timeout.
	ErrUpstream = errors.New("nzbget: upstream request failed")

	// ErrServer is a JSON-RPC error envelope returned by NZBGet:
	// {"error":{"code":...,"message":"..."}}. The wrapped error
	// includes the upstream message verbatim.
	ErrServer = errors.New("nzbget: server reported error")

	// ErrNotFound is returned when a status or lifecycle lookup by
	// NZBID has no matching item in either listgroups or history.
	ErrNotFound = errors.New("nzbget: NZBID not found")

	// ErrMissingNZBID is returned by Add when NZBGet accepts the
	// request but does not echo back an NZBID. NZBGet 17+ returns
	// the new NZBID directly from append(); older builds returned a
	// boolean that we cannot map to an item ID.
	ErrMissingNZBID = errors.New("nzbget: append did not return an NZBID")

	// ErrMalformedNZB is surfaced when the user-supplied AddRequest
	// is missing both NZBURL and RawBytes.
	ErrMalformedNZB = errors.New("nzbget: NZB payload is missing or malformed")

	// ErrConfig is returned by the factory when the persisted
	// Definition.Config blob cannot be parsed or required fields
	// (host, username, password) are absent.
	ErrConfig = errors.New("nzbget: invalid client config")
)
