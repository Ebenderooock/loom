// Package deluge implements the "deluge" download client kind. It
// targets the Deluge Web UI JSON-RPC endpoint (POST /json) rather
// than the native daemon RPC, because the Web UI is what most
// operators have exposed and it proxies to the configured daemon
// transparently.
//
// Importing the package for its side-effect registers the kind with
// the downloads core:
//
//	import _ "github.com/loomctl/loom/internal/downloads/deluge"
//
// The Client type satisfies downloads.DownloadClient and is normally
// constructed by the package factory rather than directly. Outbound
// HTTP traffic flows through the same TransportProvider /
// RateLimitProvider layering used by indexers, so per-client proxy
// and throttle configuration apply uniformly.
//
// Authentication uses a single-string-password handshake against
// auth.login; the resulting _session_id cookie is stored in the
// http.Client's cookie jar and refreshed automatically on expiry.
package deluge
