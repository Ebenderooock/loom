// Package newznabserver implements Loom's Newznab/Torznab-compatible
// aggregator HTTP surface.
//
// # What it is
//
// External clients (Radarr, Sonarr, Lidarr, Readarr, anything that
// already speaks Prowlarr) point at Loom the same way they point at
// Prowlarr today: a single base URL and an API key. Loom answers the
// standard Newznab `t=caps` / `t=search` / `t=movie` / `t=tvsearch` /
// `t=music` / `t=book` queries, fans the search out across every
// enabled indexer registered with internal/indexers, and renders the
// merged results as RSS 2.0 with the `<newznab:attr>` and
// `<torznab:attr>` extensions.
//
// # Endpoints
//
// The Server mounts two equivalent paths in the parent chi router:
//
//   - GET /api                  — the canonical Newznab path, what
//     Prowlarr-aware clients expect by default.
//   - GET /api/v1/aggregate     — the Loom-namespaced alias, useful
//     when the operator wants to make the wire-compat surface
//     explicit in dashboards or proxy rules.
//
// Both paths are handled by the same code path and produce identical
// XML; ADR-0011 records why we ship both.
//
// # Authentication
//
// The handler validates the `apikey` query parameter against the same
// API-key store the rest of Loom uses (internal/auth). Unauthenticated
// requests return Newznab error XML, not Loom's JSON envelope, because
// Newznab clients only know how to surface the upstream-shaped error.
//
// # Partial results
//
// Search fan-out uses the same partial-results semantics as
// internal/indexers.Service.Search: a single failing indexer is
// recorded in the structured log but does not abort the response.
// Clients see the union of every indexer that answered in time, with
// items ordered newest first. This matches Prowlarr's behaviour and
// keeps Radarr/Sonarr searches resilient to a flaky tracker.
//
// # What this package does NOT do
//
// This is the server-side producer of Newznab/Torznab. The matching
// client-side parser lives in internal/indexers/newznab. They share
// types via the parent indexers package but stay in separate
// packages so client and server can evolve independently.
package newznabserver
