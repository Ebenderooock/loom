# Loom Web UI documentation

User-facing documentation for the Loom React frontend (under
[`web/`](../../web/README.md)).

| Page | What it covers |
|---|---|
| [Indexers & Proxies UI](indexers-ui.md) | Manage indexers and proxies, attach proxies, run manual searches, interpret health badges. |

See also:

- [`web/README.md`](../../web/README.md) — frontend dev/build commands.
- [ADR-0009 — Indexers & Proxies (backend)](../adr/0009-indexer-proxies.md).
- [ADR-0010 — Indexers & Proxies UI](../adr/0010-indexers-and-proxies-ui.md).

## Viewing and adding titles

Movie, TV, and artist search, Discover, and person filmographies use the same
title experience. Select a title or its artwork to view full details without
adding it. Select **Add** to open library settings directly, or **Add to library**
from details to reach the same form. Review the destination library, quality
profile, monitoring, and search-on-add choices before saving. TV retains episode
monitoring, series type, and season folders; music retains audio quality and
metadata profiles.

Discover prefills settings from the source list visibly and lets you change
them for this add without modifying the list. If source tracking fails after
the add succeeds, use **Retry source tracking**; the title is not added twice.
Items lacking a provider identity cannot be safely resolved and are not added
by title matching.

Artist details show the available albums; expand an album to view editions and
tracks. Requests use the same read-only details, but retain the **Request**
action, quotas, and approval process instead of direct library addition.
