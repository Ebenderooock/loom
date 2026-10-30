# ADR-0032: Loom-owned mixed-media collections

- Status: Accepted
- Date: 2026-10-30
- Deciders: @loom-maintainers

## Context

TMDB movie sets alone cannot describe mixed movie/show universes. Equivalent
provider lists are not necessarily the same grouping. Collections must preserve
user additions/exclusions across refreshes and manual merges without conflating
library membership, monitoring, or actual files.

## Decision

Introduce an independent collections module with Loom UUID identities and a
standalone destination. Store bounded collection aggregates containing members,
current and historical source provenance, local overrides and exclusions.
Enforce source ownership relationally by provider/kind/external-ID and retain
redirects for merged collection IDs. Use transactions and optimistic revisions
for aggregate writes and merges on both database engines.

Only exact external identifiers, scoped by movie/show, deduplicate members.
Only exact source references automatically identify collections. Neither names
nor overlap infer collection equivalence. Explicitly confirmed manual merges
combine sources/members and preserve local intent. Provider refresh replaces
only that source's membership; failures leave previous successful membership
intact and record visible safe errors.

Reuse metadata clients and existing import-list ingestion rather than treating
collections as acquisition lists. Reuse shared external title details/add and
normal library management sheets rather than introducing collection-specific
media settings or acquisition controls.

## Consequences

### Positive

- One collection can hold many simultaneous TMDB/IMDb/TVDB references.
- Local intent, provenance, source errors and stale links survive restarts.
- Library acquisition and monitoring remain explicit, separately authorized.

### Negative / trade-offs

- Bounded JSON aggregate updates rewrite one collection document. This keeps
  reconciliation/merge atomic and simple; indexed per-member analytics would
  justify separate member tables later, not speculative infrastructure now.
- Provider access and identity coverage vary. In particular, TMDB cannot map
  TVDB movie IDs; unsupported/unavailable mappings must remain explicit.
- Candidate discovery and refresh are on-demand and bounded, not guarantees of
  franchise completeness.

### Neutral

- Legacy read-only Discover TMDB movie-set endpoints remain compatible.

## Alternatives considered

- Import-list ownership: conflates grouping with acquisition and cannot model
  several simultaneous source identities without losing local intent.
- Name/overlap matching: incorrectly combines distinct edits, releases, universes
  and similarly named collections.
- A new generic graph/identity framework: unnecessary scope for movie/show
  collections and harder to verify than bounded aggregate transactions.
