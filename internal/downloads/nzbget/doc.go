// Package nzbget implements the "nzbget" download-client kind.
//
// NZBGet exposes both XML-RPC and JSON-RPC at the same control port.
// Loom speaks **JSON-RPC** exclusively: a single endpoint at
// <scheme>://<host>:<port><base_path>jsonrpc that accepts
// {"jsonrpc":"2.0","method":...,"params":[...],"id":1} bodies and
// authenticates via HTTP Basic Auth using the operator's
// ControlUsername / ControlPassword pair.
//
// The kind composes its outbound transport through
// downloads.TransportProvider so the per-client proxy + throttle
// stack lights up automatically, mirroring the indexers and the
// other download kinds.
//
// Importing the package for its side effect registers the factory
// under downloads.KindNZBGet:
//
//	import _ "github.com/loomctl/loom/internal/downloads/nzbget"
//
// The factory parses the kind-specific config blob (Host, Port, TLS,
// BasePath, Username, Password) from Definition.Config, builds an
// HTTP client wrapping the composed transport, and returns a
// *Client that satisfies downloads.DownloadClient.
package nzbget
