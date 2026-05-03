// Package downloads is the download-clients subsystem of Loom.
//
// A download client is one place Loom can hand a release off to so the
// bytes actually get fetched. Examples include qBittorrent and
// Transmission for torrents, SABnzbd and NZBGet for Usenet, plus the
// builtin/null stub used for tests and dry-runs.
//
// The package is the dual of internal/indexers: where indexers ask
// "do you have anything matching this?", download clients answer
// "please go get this for me". Both share the same shape — a small
// per-kind interface, an in-memory Registry of live instances, an
// engine-neutral Repository, a Service that ties them together, and
// a HealthChecker driven by the persistent scheduler.
//
// # Adding a new kind
//
// 1. Implement the DownloadClient interface in internal/downloads/<kind>/.
//
// 2. In your package's init(), call downloads.RegisterKind(KindFoo, factoryFn).
//
// 3. Document the expected shape of config_json in your kind's package
// godoc; the downloads package treats it as opaque bytes.
//
// # Threading
//
// Registry and Repository are safe for concurrent use. Each
// DownloadClient implementation is responsible for its own internal
// synchronisation; the interface contract is that every method must
// be safe to call concurrently on a single value.
package downloads
