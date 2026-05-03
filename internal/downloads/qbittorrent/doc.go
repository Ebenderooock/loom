// Package qbittorrent implements the "qbittorrent" download client
// kind. It speaks the qBittorrent v2 Web API (qBittorrent 4.1+ and
// 5.x) using cookie-based authentication via /api/v2/auth/login.
//
// Importing the package for its side-effect registers the kind with
// the downloads core:
//
//	import _ "github.com/loomctl/loom/internal/downloads/qbittorrent"
//
// The Client type satisfies downloads.DownloadClient and is normally
// constructed by the package factory rather than directly. Outbound
// HTTP traffic flows through the same TransportProvider /
// RateLimitProvider layering used by indexers, so per-client proxy
// and throttle configuration apply uniformly.
package qbittorrent
