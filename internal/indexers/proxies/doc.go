// Package proxies implements per-indexer outbound proxies (Phase 2e).
//
// A proxy row is a first-class resource that an indexer can attach
// to via its proxy_id column. When an indexer factory builds a live
// instance it asks the package's TransportProvider (registered on
// the indexers package via indexers.SetTransportProvider) for an
// http.RoundTripper; the returned transport routes outbound HTTP
// through the configured upstream — a generic http(s) CONNECT
// proxy, a SOCKS5 proxy, or a FlareSolverr endpoint that solves
// Cloudflare interstitials by driving a headless browser.
//
// The package exposes the following surface:
//
//   - Repository: persistence over the sqlc-generated proxy queries.
//   - Service:    CRUD + connectivity testing + transport caching.
//   - Mount:      attaches /api/v1/proxies/* to a chi.Router.
//
// Service plugs into indexers.ServiceOptions.RouteExtensions so the
// proxy routes share the indexer routes' auth scope without editing
// internal/server/server.go.
//
// FlareSolverr support is intentionally narrow. We synthesise
// http.Response objects from the FlareSolverr `/v1` JSON envelope
// (status/headers/body/cookies) and use it as a RoundTripper. This
// covers the indexer-search use case (one GET per query); it does
// not attempt to be a general-purpose drop-in proxy.
package proxies
