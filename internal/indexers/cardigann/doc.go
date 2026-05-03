// Package cardigann implements the Cardigann YAML indexer kind.
//
// # Background
//
// Cardigann is a community project (https://github.com/Cardigann/cardigann
// and the Jackett/Prowlarr forks) that describes per-tracker indexer
// behaviour as YAML: a login flow, a search request template, and a
// set of CSS/XPath selectors that extract release rows from the
// upstream HTML response. Hundreds of public-tracker definitions ship
// with the upstream projects.
//
// Loom embeds a *reduced-scope* compatible loader. We re-use the same
// YAML schema so operators can drop existing definitions in place,
// but we deliberately do not re-implement the entire upstream feature
// surface. See docs/indexers-cardigann.md for the supported / deferred
// matrix.
//
// # Wiring
//
// Importing this package for its side effects registers the
// `cardigann` kind with the central factory:
//
//	import _ "github.com/loomctl/loom/internal/indexers/cardigann"
//
// During package init the factory installs itself; cmd/loom blank
// imports it so a server build picks it up automatically.
//
// The factory honours per-indexer outbound proxies by consuming
// indexers.TransportForDefinition, identical to the newznab kind.
//
// # Components
//
//   - definition.go    — Go structs that mirror the YAML schema.
//   - loader.go        — read + validate definitions from a directory.
//   - engine.go        — runtime: HTTP login, search, selector pipeline.
//   - kind.go          — registration with the central factory.
//
// # Trust model
//
// A Cardigann definition tells Loom which URLs to hit and what
// credentials to send. Only operators with access to the data dir can
// add definitions, and the API surface accepts only a definition_id
// (path) — never raw YAML over the wire. Definitions never execute
// arbitrary code; the only "scripting" is selector / filter
// evaluation, which is bounded by the engine.
package cardigann
