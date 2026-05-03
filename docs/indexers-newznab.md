# Newznab + Torznab indexers

Loom ships a first-class client for the Newznab (Usenet) and Torznab
(BitTorrent) protocols. They register under the kinds `newznab` and
`torznab` respectively. The client lives at
`internal/indexers/newznab` and is wired automatically when the binary
starts — no extra packages, no plugin dance.

This document covers configuration, common operator setups, the curl
surface, the caps caching flow, and troubleshooting tips.

## When to use which kind

| Kind       | Wire format                | Typical front-end                                            |
| ---------- | -------------------------- | ------------------------------------------------------------ |
| `newznab`  | RSS + `xmlns:newznab` attr | Prowlarr → Newznab indexer, NZBHydra2 (`api?t=newznab`)      |
| `torznab`  | RSS + `xmlns:torznab` attr | Prowlarr → Torznab indexer, Jackett, NZBHydra2 (`/torznab/`) |

Both kinds share the same envelope; the only runtime difference is
which extended-attribute namespace gets parsed and which output fields
(grabs/files/group vs seeders/peers/infohash) come through.

## Configuration

The `config` JSON blob accepts these keys:

| Key            | Required | Default                | Notes                                                                    |
| -------------- | -------- | ---------------------- | ------------------------------------------------------------------------ |
| `url`          | yes      | —                      | Indexer base URL (e.g. `https://nzbhydra.example/api`).                  |
| `api_key`      | yes      | —                      | Per-indexer API key. May be supplied as `?apikey=` in `url` instead.     |
| `user_agent`   | no       | `Loom/0.1 (+https://…)`| Sent on every outbound request.                                          |
| `timeout`      | no       | `30s`                  | Go duration string. Applied to caps + search HTTP calls.                 |
| `category_map` | no       | —                      | Optional named alias → list of upstream IDs; reserved for a later phase. |

The client is forgiving on the way in:

- A trailing `/` on `url` is stripped.
- An `?apikey=...` embedded in `url` is extracted into `api_key` if
  the field was empty.

## Quick setup

### Prowlarr (most common)

1. In Prowlarr, add the indexer you want to expose.
2. Click **Apps** → **Loom** is not yet a first-class consumer, so
   copy the Prowlarr-generated **API Key** and the per-indexer URL
   (typically `http://prowlarr:9696/<n>/api`).
3. Create the indexer in Loom (see curl below).

### NZBHydra2

1. In NZBHydra2, the per-indexer URL is `https://hydra/api`.
2. The API key is the global key from **Settings → Authorization**.
3. Create the indexer in Loom; pick `newznab` for the Usenet feed and
   `torznab` for the torrent feed.

### Jackett

Jackett URLs look like `http://jackett:9117/api/v2.0/indexers/<slug>/results/torznab/api`.
Use kind `torznab`.

## Curl examples

Create a Newznab indexer:

```bash
curl -X POST http://localhost:8088/api/v1/indexers \
  -H 'Authorization: Bearer $LOOM_TOKEN' \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "nzbhydra-news",
    "kind": "newznab",
    "name": "NZBHydra2 (Newznab)",
    "enabled": true,
    "priority": 50,
    "config": {
      "url": "https://nzbhydra.example/api",
      "api_key": "abcdef0123456789",
      "timeout": "20s"
    },
    "categories": [2000, 5000, 7000]
  }'
```

Create a Torznab indexer:

```bash
curl -X POST http://localhost:8088/api/v1/indexers \
  -H 'Authorization: Bearer $LOOM_TOKEN' \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "prowlarr-torrents",
    "kind": "torznab",
    "name": "Prowlarr (Torznab)",
    "enabled": true,
    "priority": 40,
    "config": {
      "url": "http://prowlarr:9696/1/api",
      "api_key": "...",
      "timeout": "30s"
    },
    "categories": [2000, 5000]
  }'
```

Probe caps + run a TV search:

```bash
curl http://localhost:8088/api/v1/indexers/nzbhydra-news/caps
curl 'http://localhost:8088/api/v1/indexers/nzbhydra-news/search?q=showname&season=2&tvdb_id=12345'
curl -X POST http://localhost:8088/api/v1/indexers/nzbhydra-news/test
```

## Caps cache lifecycle

The first call to `Caps()` (or `Test()`) issues `GET <url>?t=caps` and
stores the parsed `Caps` JSON in `indexer_health.last_caps_json`. On
subsequent restarts:

1. `cmd/loom` builds a `CapsCache` for the active engine (SQLite or
   Postgres) and registers it with `newznab.SetCapsCache`.
2. When a `Client` is built (during `Service.HydrateAll`), it loads
   the cached document so the next API request returns instantly
   without a network call.
3. Every successful caps fetch overwrites the cached row. Failures
   are not cached.

If you need a forced refresh, `POST /api/v1/indexers/{id}/test`
re-runs the caps round-trip and overwrites the cache.

## Search routing

The internal `Query` shape gets mapped onto a Newznab mode at request
time:

| Query has…                 | Mode chosen | Upstream `t=` |
| -------------------------- | ----------- | ------------- |
| `imdb_id` or `tmdb_id`     | movie       | `movie`       |
| `tvdb_id`, `season`, `ep`  | tvsearch    | `tvsearch`    |
| Otherwise                  | search      | `search`      |

Categories (`Query.Categories`) are joined as a comma-separated
`cat=...` parameter. `limit` becomes `limit` and `offset` is currently
fixed at `0` (a later phase adds pagination).

## Troubleshooting

| Symptom                                                            | Likely cause + fix                                                                                                                       |
| ------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `Test` returns `newznab: auth failed: code=100`                    | Wrong `api_key`. Double-check it on the upstream. Loom strips an embedded `?apikey=` from the URL — set `api_key` explicitly to be sure. |
| `Test` returns `newznab: malformed xml`                            | The upstream sent HTML (e.g. a Cloudflare interstitial). Verify the URL is reachable and not behind a captcha.                           |
| Health flips to `degraded` after a sweep                           | Upstream returned `429 Too Many Requests`. Loom will keep the indexer enabled but back off; raise the upstream's rate limit if you can.  |
| Search returns 0 items but caps shows the mode is available        | Some indexers reject `tvsearch` without a `tvdbid`. Provide one in the search query, or fall back to plain `q`.                          |
| Torrent results have an empty `quality` field that holds an infohash | Phase 2c stashes the torznab infohash on `Result.Quality` because the `Result` struct does not yet have a dedicated infohash field. ADR-0008 documents the carve-out and a later phase will add a proper field. |

## See also

- [Indexer abstraction](indexers.md) — the core `Definition`,
  `Indexer`, and registry concepts that this kind plugs into.
- [API reference](api.md) — full HTTP surface of `/api/v1/indexers`.
- [ADR-0008](adr/0008-newznab-torznab-client.md) — design rationale.
