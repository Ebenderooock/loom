# ADR-0011: Newznab/Torznab aggregator server

- Status: Accepted
- Date: 2025-01-14
- Deciders: @loom-maintainers

## Context

Phase 2c gave Loom Newznab and Torznab *clients*: it can talk to
upstream indexers that expose those wire formats. Phase 2d turns the
relationship around — Loom needs to *speak* Newznab/Torznab so that
external clients (Sonarr, Radarr, Lidarr, Readarr, and Prowlarr-aware
tooling in general) can treat one Loom instance as a single
aggregating "indexer" and stop being configured against each upstream
individually.

The wire surface Prowlarr exposes is well established:

- `GET /api?t=caps&apikey=…` returns a `<caps>` document advertising
  the aggregator's search modes, supported external IDs, and
  category set.
- `GET /api?t=search|movie|tvsearch|music|book&apikey=…` returns an
  RSS 2.0 channel with one `<item>` per result and `<newznab:attr>`
  (Usenet) or `<torznab:attr>` (torrent) extension elements carrying
  the fields RSS itself can't express.
- Errors are RSS-shaped XML documents: `<error code="…"
  description="…"/>` with a small set of well-known codes.

Loom already has a parallel JSON surface at `/api/v1/indexers/*` for
its own UI, and an API-key store backing both the JSON API and the
upcoming wire-compat layers. The aggregator must reuse the existing
key store but surface failures in Newznab's XML format instead of
Loom's JSON `{error: …}` envelope.

## Decision

Ship the aggregator as a new package `internal/indexers/newznabserver`
that:

1. **Renders RSS 2.0 with newznab/torznab extensions** by hand using
   `encoding/xml`. The package owns its own envelope/item/attr structs
   and chooses the namespace per-item: torrent-flavoured results
   (non-empty Infohash, non-nil Seeders/Peers, or a magnet URI) emit
   `<torznab:attr>`, everything else emits `<newznab:attr>`. Both
   namespaces are declared on the root `<rss>` element so a feed may
   mix the two — useful when a single Loom instance fronts Usenet and
   torrent indexers simultaneously.
2. **Aggregates caps as a union, not an intersection.** A search mode
   (`tv-search`, `movie-search`, etc.) is advertised when *any*
   registered indexer reports it. A category appears when any indexer
   declares it. Supported external IDs (`imdbid`, `tvdbid`, `tmdbid`)
   likewise union. This matches Prowlarr's behaviour and avoids the
   degenerate empty caps you'd get from intersecting heterogeneous
   indexers.
3. **Authenticates via `?apikey=` query parameter** against the
   existing `auth.Service` API key store, falling back to
   `X-Api-Key` for the (rare) clients that use it. Auth lives in the
   handler itself rather than via `auth.Service.RequireAuth`
   middleware: `RequireAuth` writes JSON errors, but Newznab clients
   parse only XML. We added a small public `VerifyAPIKey(ctx, key)
   error` method to `auth.Service` for this purpose; it returns
   `ErrUnauthenticated` on every failure mode so the credential state
   does not leak.
4. **Mounts at two paths:** `/api` (the canonical Prowlarr-compat
   path that Sonarr/Radarr/Lidarr default to) and
   `/api/v1/aggregate` (a Loom-namespaced alias for operators that
   already reverse-proxy `/api/v1/*` and want the aggregator to live
   in the same prefix). Both routes resolve to the same handler. The
   chi router dispatches `/api` exact-match before falling through
   to the JSON `/api/v1/*` group, so there is no collision.
5. **Treats per-source failures as partial success.** When the
   underlying `*indexers.Service.Search` returns errors keyed by
   indexer ID alongside a result list, the aggregator logs the errors
   and returns the results. Newznab has no concept of partial-success
   metadata in a feed, and Sonarr/Radarr clients treat any 5xx as
   "indexer down — back off". Returning a partial feed is strictly
   better than failing the whole request.
6. **Uses the package as the only entry point.** No package-level
   globals; `NewServer(Options{…})` returns a `*Server`,
   `Mount(router)` registers the routes. This makes the aggregator
   trivially substitutable in tests and keeps the existing
   `internal/server.Server` constructor's wiring uniform.

## Consequences

### Positive

- One Loom URL, one API key — Sonarr/Radarr/Lidarr operators no
  longer configure each upstream indexer separately.
- The aggregator inherits every transport policy already wired into
  `*indexers.Service`: per-indexer timeouts, parallelism cap,
  proxy/retry/backoff configured per indexer.
- Adding a new search mode or a new attr namespace is a localised
  change in `feed.go` / `caps.go`; the rest of the indexer stack
  stays untouched.

### Negative / trade-offs

- Hand-rolled XML namespacing. Go's `encoding/xml` does not have
  ergonomic support for prefixed local names; we set
  `XMLName: xml.Name{Local: "torznab:attr"}` and rely on Go preserving
  the colon verbatim. This is the canonical workaround in the wild
  but is brittle if `encoding/xml` ever stops accepting colons in
  local names. Tests assert the literal byte output to catch any
  future regression.
- Caps aggregation hides per-indexer divergence. A client cannot tell
  "this Loom instance has one Usenet and one torrent backend" from
  the caps document alone. Operators who care can scope queries to a
  specific indexer via the existing JSON `/api/v1/indexers/search`
  route, which still accepts an `indexer_ids` filter.
- Auth state shape leaks "valid" vs "invalid" but never the reason
  (expired vs disabled vs unknown). This is a deliberate choice — the
  Newznab error code namespace is small and Sonarr/Radarr never
  surface the description verbatim, so there is no operator value in
  finer-grained codes.

### Neutral

- The aggregator only emits XML. JSON is not part of the Newznab
  contract and is not requested. If a future client needs a JSON
  view of the same data the existing `/api/v1/indexers/search`
  endpoint already serves it.

## Alternatives considered

- **Mount as middleware on the JSON indexer surface.** Rejected:
  `auth.Service.RequireAuth` writes JSON errors and we'd need a
  parallel "if Accept: application/xml then…" branch, which would
  bind two unrelated wire formats together at the framework layer.
- **Return per-indexer feeds at `/api/<id>?t=…`.** Rejected: this
  defeats the aggregator goal — clients would need to know the IDs in
  advance, which is the configuration burden we want to eliminate.
- **Use a third-party library (e.g. `gorilla/feeds`).** Rejected: no
  current Go RSS library supports `xmlns:newznab`/`xmlns:torznab`
  extension elements out of the box, and adding the dependency just
  to skip 200 lines of XML structs is poor stewardship.
