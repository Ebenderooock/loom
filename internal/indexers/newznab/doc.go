// Package newznab implements the Newznab and Torznab outbound client
// kinds for Loom.
//
// # Wire protocols
//
// Newznab and Torznab are HTTP+XML query protocols:
//
//   - Newznab originated in the Usenet world (NZBmatrix-era). Servers
//     answer GET requests at a single endpoint with a `t=` mode
//     parameter; results come back as RSS 2.0 channel/items decorated
//     with a `<newznab:attr>` namespace for fields the spec doesn't
//     give first-class slots (group, files, grabs, ...).
//   - Torznab is Jackett's BitTorrent variant of the same shape:
//     identical envelope, but a `<torznab:attr>` namespace adds
//     `seeders`, `peers`, `infohash`, `magneturl`, etc.
//
// Loom treats the two as one implementation parameterised by which
// attribute namespace to read; both register their own kind string so
// definitions and the OpenAPI spec stay self-documenting.
//
// # Wiring
//
// Importing this package for its side effects registers two kinds in
// the central catalogue:
//
//	import _ "github.com/loomctl/loom/internal/indexers/newznab"
//
// During package init the factories install themselves under
// "newznab" and "torznab"; cmd/loom blank-imports here so a server
// build picks both up automatically.
//
// # Caps caching
//
// `Caps()` returns a snapshot. The first call after process start
// fetches `t=caps` from upstream, parses it, stores it in memory, and
// also persists the JSON to indexer_health.last_caps_json via the
// shared CapsCache so a restart can serve cached caps before the
// first scheduled health check fires. The cache is refreshed on every
// `Test()` invocation (which the scheduler drives every 10 minutes).
//
// # Errors
//
// Upstream failures are mapped to typed errors in errors.go so HTTP
// handlers and the health checker can branch on category rather than
// match strings.
package newznab
