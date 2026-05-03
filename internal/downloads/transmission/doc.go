// Package transmission implements the "transmission" download-client
// kind. It speaks the Transmission RPC protocol (Transmission 3.x and
// newer, where labels were promoted to first-class metadata) by
// POSTing JSON-RPC envelopes to /transmission/rpc.
//
// Importing the package for its side effect registers the factory
// under downloads.KindTransmission:
//
//	import _ "github.com/loomctl/loom/internal/downloads/transmission"
//
// Transmission's RPC layer enforces a CSRF mitigation: every request
// must carry a recently-issued X-Transmission-Session-Id header.
// Stale (or absent) ids elicit a 409 Conflict whose response carries
// the new id. The Client transparently captures the header and
// replays the request once on every 409, so callers never see the
// handshake.
//
// Outbound HTTP traffic flows through the same TransportProvider /
// RateLimitProvider layering used by indexers and the other download
// kinds, so per-client proxy and throttle policies apply uniformly.
package transmission
