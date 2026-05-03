# ADR-0012: Cardigann YAML definition loader

- Status: Accepted
- Date: 2025-01-XX
- Deciders: @loom-maintainers

## Context

Loom needs broad coverage of private trackers without writing one Go
client per tracker. The de-facto standard for tracker portability is
Cardigann's YAML definition format, used (with local extensions) by
Jackett and Prowlarr. A definition declares the site's login flow,
search request shape, result-table CSS/XPath, and a small filter
chain for cleaning extracted strings — enough to drive a generic
engine without per-site code.

Two questions had to be answered:

1. **Trust boundary.** YAML on disk is fine; YAML over the API is
   not — operators would inevitably paste hostile fragments and a
   selector engine that runs anything from anywhere is a much bigger
   attack surface than the rest of Loom.
2. **Library choice.** We need both CSS selectors (Cardigann's
   default) and XPath (used by some definitions for table-row
   walking).

## Decision

Implement a `cardigann` indexer kind with these properties:

- A package-global `Loader` scans `<data_dir>/definitions/cardigann/`
  recursively at boot, validates each file, and exposes
  `Get(id)`/`All()`. The kind's factory looks the definition up by
  `definition_id` (the file's basename) — the API never accepts an
  inline definition.
- Selector dispatch is automatic: a selector starting with `/` or
  `(` runs through [antchfx/htmlquery](https://github.com/antchfx/htmlquery)
  (XPath); everything else runs through
  [PuerkitoBio/goquery](https://github.com/PuerkitoBio/goquery) (CSS).
- The filter chain implements the most-used filters
  (`replace`, `trim`, `regexp`, `querystring`, `prepend`, `append`,
  `split`, `lowercase`, `uppercase`, `join`); unknown filter names
  are pass-throughs rather than errors so a tracker definition with
  one unsupported step still produces usable rows.
- Login modes implemented: `form` / `post` (alias) and `cookie`. A
  pre-baked `name=value; name2=value2` cookie string in the indexer
  config seeds the cookie jar at engine construction.
- Categories: definitions declare a `categorymappings` table mapping
  site IDs to Newznab category names (e.g. `Movies/HD`); the kind
  ships a name → numeric lookup table covering the standard family
  IDs and the common subdivisions. Operators can override individual
  mappings via `category_overrides` in the indexer config.

## Consequences

### Positive

- Adding support for a new tracker becomes a YAML drop, not a code
  change.
- Reusing the upstream schema makes it easy to source-of-truth check
  definitions against Jackett / Prowlarr; operators can copy
  definitions over with minimal edits.
- The trust model stays simple: there is exactly one path for
  loading definitions (the data dir) and exactly one role
  (operator-with-shell) that can place files there.

### Negative / trade-offs

- Schema drift risk: upstream Cardigann adds fields and filters
  faster than we can chase them. Our YAML decoder uses
  `KnownFields(false)` so unknown fields are silently ignored, and
  unknown filter names are pass-throughs — definitions degrade
  rather than fail.
- The filter set is intentionally smaller than upstream's. Some
  trackers may need a follow-up phase to add (e.g.) `dateparse`,
  `urlencode`, or `htmldecode`.
- Each request walks the full HTML; we do not currently cache parsed
  documents across pages of a single search. Acceptable while
  multi-path searches are out of scope.

### Neutral

- We honour per-indexer outbound proxies via the same
  `indexers.TransportForDefinition` callback the `newznab` kind uses,
  so the proxies subsystem applies uniformly.
- Definition IDs come from the filename, not the YAML's `site:`
  field. This deviates from upstream (which keys by `site:`) but
  lets operators run two flavours of the same tracker side by side
  — useful for staging vs production credentials.

## Alternatives considered

- **Per-tracker Go clients.** Maintains a clear contract per
  tracker, but the maintenance burden scales with the tracker count
  and we do not want to be the bottleneck for adding a new private
  tracker.
- **Embedding Cardigann directly.** Cardigann the binary is a
  full-blown Newznab façade; embedding it would import a process
  model and scheduling layer we do not need. We chose to consume
  the schema, not the runtime.
- **Inline-YAML API submission.** Operationally tempting (paste-and-go
  in the UI) but would either require sandboxing the selector
  engine or trusting authenticated callers with arbitrary regex /
  XPath against arbitrary HTML. Not worth the surface for the small
  convenience win — the data-dir route stays.

## See also

- [docs/indexers-cardigann.md](../indexers-cardigann.md) — operator-facing
  guide, supported / deferred matrix, debugging tips.
- [ADR-0007](0007-indexer-abstraction.md) — Indexer interface.
- [ADR-0009](0009-indexer-proxies.md) — per-indexer transport routing.
