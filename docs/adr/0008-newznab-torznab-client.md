# ADR-0008: First-class Newznab + Torznab client

- Status: Accepted
- Date: 2025-09-02
- Deciders: @loom-maintainers

## Context

Phase 2a introduced the indexer abstraction (`internal/indexers`) with
a single `builtin/null` kind. Real users need to talk to existing
Newznab (Usenet) and Torznab (BitTorrent) feeds — typically front-ends
like Prowlarr, NZBHydra2, or DIY definitions — without waiting for a
Cardigann compiler.

Both protocols share an envelope (RSS 2.0 with extended attributes)
and only differ in the namespace of the `<*:attr>` elements
(`xmlns:newznab` vs `xmlns:torznab`) and the operator-relevant fields
they surface (Newznab → `grabs`, `files`, `group`; Torznab → `seeders`,
`peers`, `infohash`).

We had three architectural choices:

1. Re-use a third-party Go newznab library;
2. Embed Cardigann's TOML/YAML definitions and compile them at runtime;
3. Write a single-purpose client in-tree that implements
   `indexers.Indexer` directly.

We also needed to decide how to persist the caps document so a restart
doesn't blank-state every indexer. The Phase 2a `Repository` is
deliberately read-mostly and `repository.go` is on the
"don't-modify-without-RFC" list.

## Decision

We ship a hand-written client at `internal/indexers/newznab` that
registers two kinds, `newznab` and `torznab`, and embeds the parser,
search router, and HTTP plumbing. Both kinds share `Client`; the only
runtime difference is `attrFlavour`, an unexported enum read by
`mapItem` to choose between `mapNewznabAttrs` and `mapTorznabAttrs`.

Caps caching is implemented through a new `CapsCache` interface in
`internal/indexers/caps_cache.go` with SQLite and Postgres
implementations. The `last_caps_json` column was added to
`indexer_health` via migration `0007_indexer_caps_cache.sql` (TEXT on
SQLite, JSONB on Postgres). Reads happen at `NewClient` time, writes
happen on every successful `Test()` / lazy `Caps()` fetch. Existing
`UpsertIndexerHealth` SQL only sets the columns it knows about, so the
new column is preserved across health writes.

Errors are typed (`ErrAuthFailed`, `ErrCapsParse`, `ErrRateLimited`,
`ErrUpstream`, `ErrTimeout`, `ErrMalformedXML`) so the
`HealthChecker` can promote 429 to `degraded` rather than `failed`,
and the API surface can map auth failures consistently.

## Consequences

### Positive

- No third-party Newznab/Torznab parser dependency; the client is
  ~600 lines of standard-library Go.
- A single `Client` implementation services both kinds, reducing
  drift.
- Caps cached to disk means `GET /api/v1/indexers/{id}/caps` is fast
  on first call after a restart.
- Typed errors give the rest of the system a stable taxonomy to
  branch on.
- `parseConfig` tolerates two real-world operator slip-ups
  (trailing `/` on URL, embedded `?apikey=`), reducing config-error
  support load.

### Negative / trade-offs

- Sub-categories pass through as raw integers — operators wanting
  curated families (e.g. "TV/HD only") rely on caller-side filters
  for now.
- The Phase 2a `Result` struct does not have a dedicated `infohash`
  field; we stash the torznab `infohash` attribute on `Result.Quality`
  with a documented carve-out. A later phase that revisits `Result`
  will give it a proper home.
- Hand-rolled clients can't replicate every quirk Cardigann handles
  (login flows, captcha, multi-page torrent sites). That gap is the
  motivation for Phase 2d Cardigann support — newznab is intentionally
  the "easy" case.

### Neutral

- The `CapsCache` interface lives in the indexers package rather than
  the kind package because more than one kind may need it (Cardigann
  in Phase 2d will likely reuse it).

## Alternatives considered

- **Wrap an existing library.** Most Go newznab libraries are
  abandoned, panic on malformed XML, or pull in transitive
  dependencies (logrus, color terminals, etc.) that conflict with our
  baseline.
- **Compile Cardigann definitions in-tree.** Larger surface, slower
  iteration. Phase 2d will add this in addition to, not instead of,
  the first-class client.
- **Cache caps in-memory only.** Simple, but every restart re-spends
  network round-trips and the operator-facing latency on the first
  search after a restart spikes. The `last_caps_json` column is one
  cheap field.
