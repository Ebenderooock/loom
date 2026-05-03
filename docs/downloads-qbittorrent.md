# qBittorrent download client

> Configure Loom to dispatch torrent grabs to a qBittorrent instance.

This guide is operator-facing. For the design rationale, see
[ADR-0015](adr/0015-qbittorrent-download-client.md).

## Supported versions

- **qBittorrent 4.1 and later.** The v2 Web API landed in 4.1.0; older
  releases are not supported.
- **qBittorrent 5.x.** Supported as-is. Loom does not depend on any
  v5-only endpoints.

The `Test` round-trip hits `/api/v2/app/version` so an unsupported
build surfaces clearly in the health row.

## Configuration

Create a download client through the API or the web UI. The kind
string is `qbittorrent`. Required `config` fields:

| Field       | Type    | Default | Notes                                                            |
| ----------- | ------- | ------- | ---------------------------------------------------------------- |
| `host`      | string  |         | Hostname or IP of the qBittorrent Web UI.                        |
| `port`      | integer |         | TCP port (e.g. `8080`).                                          |
| `tls`       | boolean | `false` | `true` when the Web UI is served over HTTPS.                     |
| `base_path` | string  | `/`     | URL prefix when qBittorrent runs behind a reverse-proxy subpath. |
| `username`  | string  |         | Web UI username.                                                 |
| `password`  | string  |         | Web UI password (write-only — never returned on GET).            |

`host`, `port`, `tls`, `username`, and `password` may also be set on the
parent download-client row; values inside the `config` blob win when both
are present so operators can keep secrets in one place.

### Worked example

```bash
curl -sS -H "X-Api-Key: $LOOM_API_KEY" \
     -H "Content-Type: application/json" \
     -d '{
       "name": "Home qBittorrent",
       "kind": "qbittorrent",
       "protocol": "torrent",
       "enabled": true,
       "category_default": "loom",
       "save_path_default": "/downloads/loom",
       "config": {
         "host": "qbittorrent.lan",
         "port": 8080,
         "tls": false,
         "base_path": "/",
         "username": "admin",
         "password": "adminadmin"
       }
     }' \
     http://localhost:7878/api/v1/download-clients/
```

Test the connection:

```bash
curl -sS -X POST -H "X-Api-Key: $LOOM_API_KEY" \
     http://localhost:7878/api/v1/download-clients/$ID/test
```

## Categories

qBittorrent categories double as save-path overrides. Loom reads them
through `GET /api/v1/download-clients/{id}/categories`, which proxies
`/api/v2/torrents/categories`. Create categories from the qBittorrent
web UI; Loom does not currently provision categories on your behalf.

When `Add` is called with a `category` field, Loom passes it through
verbatim. qBittorrent will create the category lazily if it does not
already exist (qBittorrent 4.4+); on older releases the add will
succeed but the category will be silently dropped.

## Adding torrents

Loom honours the same precedence as the `AddRequest` contract:

1. `RawBytes` — uploaded as a multipart `torrents` file part. Loom
   computes the v1 infohash from the bencoded `info` dict and returns
   it as the item id.
2. `Magnet` — submitted via the multipart `urls` field. Loom extracts
   the BTIH from the `xt=urn:btih:` parameter for the item id.
3. `TorrentURL` — submitted via `urls`. The infohash is not known until
   qBittorrent fetches and parses the file, so Loom returns an empty
   item id; subsequent `Status` calls will surface the row once
   qBittorrent reports it.

Tags supplied on the request are joined with commas and forwarded as
the `tags` field. The `paused` flag is always sent as `false` — call
`Pause` after `Add` if you want a paused-on-add behaviour.

## State mapping

Loom collapses qBittorrent's rich state vocabulary onto the small
`ItemStatus` enum:

| qBittorrent state                                            | Loom status   |
| ------------------------------------------------------------ | ------------- |
| `downloading`, `forcedDL`, `metaDL`, `checkingDL`, `stalledDL`, `allocating`, `moving`, `checkingResumeData` | `downloading` |
| `queuedDL`, `queuedUP`                                       | `queued`      |
| `uploading`, `forcedUP`, `stalledUP`, `checkingUP`           | `seeding`     |
| `pausedDL`                                                   | `paused`      |
| `pausedUP`                                                   | `completed`   |
| `error`, `missingFiles`                                      | `failed`      |
| anything else                                                | `unknown`     |

The `pausedUP` → `completed` mapping is deliberate: from the operator's
perspective a paused-while-seeding torrent is finished work, not stalled
work, even though qBittorrent itself still calls it "paused".

## Common gotchas

- **Reverse-proxy subpaths.** If you front qBittorrent with nginx /
  Traefik / Caddy on a subpath (e.g. `https://example.com/qbt/`),
  you **must** match `base_path` to that subpath and toggle the
  qBittorrent option *Bypass authentication for clients on whitelisted
  IPs* off. Loom sets `Referer` and `Origin` to the configured base URL
  on every request to satisfy qBittorrent's CSRF protection; a
  mismatched `base_path` will surface as 403s on `app/version`.
- **`Host`-header validation.** qBittorrent rejects requests whose
  `Host` header does not match the value configured in *Web UI →
  Server domains*. Either add Loom's hostname to the whitelist or set
  the field to `*`. Symptoms: `Test` returns
  `qbittorrent: app/version probe failed` with a `401` body of
  `Unauthorized`.
- **Account lockout.** qBittorrent temporarily bans the calling IP
  after several failed logins. Loom surfaces the resulting 403 as
  `authentication failed`. Restart qBittorrent or wait out the ban
  (default: 1 hour).
- **Free-space reporting.** Older 4.1.x builds and bare-metal Windows
  installs sometimes omit `free_space_on_disk` from
  `/sync/maindata`; Loom records `-1` (unknown) in those cases rather
  than failing health.

## Deferred features

These are not implemented today and are tracked for a future phase:

- Torrent priority / queue position management.
- Selective file downloading (`/torrents/files`, `/torrents/filePrio`).
- Tracker-level inspection beyond what `Status` exposes.
- Provisioning categories from Loom (only round-tripping is supported).
