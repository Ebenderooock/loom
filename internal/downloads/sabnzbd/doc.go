// Package sabnzbd implements the "sabnzbd" download-client kind.
//
// It speaks the SABnzbd JSON API (3.x and newer): a single endpoint
// at <scheme>://<host>:<port><base_path>api parameterised by mode=...
// and authenticated via the apikey query parameter. The client
// composes its outbound transport through downloads.TransportProvider
// so per-client proxy and rate-limit policies apply automatically,
// just like the Newznab indexer kind.
//
// Importing the package for its side effect registers the factory
// under downloads.KindSABnzbd:
//
//	import _ "github.com/loomctl/loom/internal/downloads/sabnzbd"
//
// The factory parses the kind-specific config blob (Host, Port, TLS,
// BasePath, APIKey) from Definition.Config, builds an HTTP client
// with the composed transport, and returns a *Client that satisfies
// downloads.DownloadClient.
package sabnzbd
