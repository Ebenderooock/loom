# Newznab/Torznab aggregator (Prowlarr-compat)

Loom can present every enabled indexer as a single Newznab/Torznab
endpoint. This is the wire format Sonarr, Radarr, Lidarr, Readarr,
and Prowlarr-aware tooling speak; pointing one of those clients at
Loom replaces a per-indexer configuration burden with one URL and one
API key.

This page covers the user-facing surface. For background on the
design decisions see [ADR-0011](adr/0011-newznab-aggregator-server.md).
For the indexer subsystem the aggregator fans out to, see
[indexers.md](indexers.md).

## Endpoint

The aggregator is mounted at two paths that resolve to the same
handler:

| Path | When to use it |
|---|---|
| `/api` | Default for Prowlarr-aware clients (Sonarr, Radarr, Lidarr). They append `?t=…&apikey=…` themselves. |
| `/api/v1/aggregate` | Alias for operators who already reverse-proxy `/api/v1/*` and want the aggregator under the same prefix. |

Both routes accept the same query parameters and emit the same XML.

## Authentication

Every call must carry a valid Loom API key. Newznab clients send the
key as a query parameter:

```
GET /api?t=caps&apikey=YOUR_KEY
```

For parity with the JSON API the aggregator also accepts an
`X-Api-Key` request header. Bearer tokens are not accepted on this
surface — Sonarr/Radarr do not send them.

A missing or invalid key yields a Newznab-shaped XML error with
HTTP 401:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<error code="101" description="invalid apikey"/>
```

When the project-level auth service is disabled (`auth.mode=disabled`
in config) the aggregator is effectively open, matching the behaviour
of the rest of Loom's HTTP surface in that mode.

## Supported `t=` modes

| Mode | What it does |
|---|---|
| `caps` | Returns the union of every indexer's capabilities as a `<caps>` document. |
| `search` | Free-text search across every enabled indexer. |
| `movie` | Movie search; honours `imdbid`, `tmdbid`. |
| `tvsearch` | TV search; honours `tvdbid`, `season`, `ep`. |
| `music` | Free-text search restricted to the Audio category family. |
| `book` | Free-text search restricted to the Books category family. |

Unknown values yield XML error code `202` ("function not
implemented") so the client surfaces a useful message rather than
404.

## Search parameters

| Param | Meaning |
|---|---|
| `q` | Free-text search term. |
| `cat` | Comma-separated list of Newznab category IDs (e.g. `2000,5040`). Non-numeric tokens are silently dropped. |
| `limit` | Per-indexer row cap. Falls through to each indexer's default when omitted. |
| `imdbid` | Movie disambiguation; only used by `t=movie`. |
| `tmdbid` | Movie disambiguation; only used by `t=movie`. |
| `tvdbid` | TV disambiguation; only used by `t=tvsearch`. |
| `season`, `ep` | TV episode targeting; only used by `t=tvsearch`. |

Unknown parameters are ignored.

## Response shape

A search returns an RSS 2.0 channel where each `<item>` is one
result. Both `xmlns:newznab` and `xmlns:torznab` are declared on the
root `<rss>` element so the feed can mix Usenet and torrent results
(typical when one Loom instance fronts both kinds of indexer).

- **Usenet results** carry their per-result metadata as
  `<newznab:attr name="…" value="…"/>` extension elements.
- **Torrent results** — anything with a non-empty infohash, a magnet
  URI, or a non-nil seeder/peer count — carry the same metadata as
  `<torznab:attr>` instead. The two namespaces are sibling, never
  nested.
- The `<enclosure>` element's `type` reflects the result kind:
  `application/x-bittorrent` for torrents,
  `application/x-nzb` for Usenet.
- The originating indexer ID is always emitted as an
  `indexer` attr so debugging the aggregator's fan-out is one
  `xmllint` invocation away.

Per-indexer failures during a fan-out are logged and dropped from the
response. Newznab clients have no partial-success concept; returning
a feed with the indexers that did answer is strictly better than
failing the request.

## Caps document

`t=caps` returns a `<caps>` document whose `<searching>` block
advertises the **union** of every indexer's modes (i.e. movie-search
is "yes" if *any* indexer supports it) and whose `<categories>`
block is the deduplicated, sorted union of every indexer's
declared categories.

This matches Prowlarr's behaviour and is what Sonarr/Radarr expect
when discovering a single aggregator they treat as their sole
indexer.

## Examples

Caps:

```bash
curl 'http://loom.local:8080/api?t=caps&apikey=YOUR_KEY'
```

TV search:

```bash
curl 'http://loom.local:8080/api?t=tvsearch&apikey=YOUR_KEY&tvdbid=121361&season=2&ep=4'
```

Movie search by IMDb ID:

```bash
curl 'http://loom.local:8080/api?t=movie&apikey=YOUR_KEY&imdbid=tt0133093'
```

## Pointing Sonarr / Radarr / Lidarr at Loom

In the client's *Settings → Indexers* page, add a new generic
Newznab (Sonarr/Radarr) or Torznab (Lidarr also accepts Newznab):

- **URL**: `http://loom.local:8080` (the aggregator handler is at
  `/api` so do *not* include the path).
- **API Key**: any active Loom API key.
- **Categories**: leave empty (the client picks them up from the
  caps document) or list the IDs you want explicitly.

The client will probe `t=caps` first, then issue searches as items
are added.

## What is intentionally not implemented

- `t=details`, `t=getnfo`, `t=register`, and the rest of the rarely
  used Newznab modes. They yield error code `202`.
- A JSON view of the same data — the JSON `/api/v1/indexers/search`
  surface is the right route for that and already exists.
- Per-indexer addressing (`/api/<id>?t=…`). Operators who need to
  scope a query to a single source can use the JSON API's
  `indexer_ids` filter.

See ADR-0011 for the reasoning behind each of these.
