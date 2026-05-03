# SABnzbd download client

> Configure Loom to hand NZBs off to a running SABnzbd 3.x or newer
> instance.

This page is the operator's guide to the `sabnzbd` download client
kind. For the design rationale, deferred features, and state-mapping
table see [ADR-0016](adr/0016-sabnzbd-download-client-kind.md).

## What you need before you start

- A running SABnzbd **3.0 or newer**. Older 2.x builds default to XML
  and will fail; the version probe (`mode=version`) doubles as the
  compatibility check.
- The SAB **API key**. Find it in SABnzbd under **Config → General →
  Security → API Key**. It is a 32-character hex string.
- Network reachability from Loom to SAB. If SAB sits behind a reverse
  proxy at a subpath (e.g. `https://media.example/sabnzbd/`), record
  the prefix — you will pass it as `base_path`.

## Configuration

The SABnzbd client is registered when Loom starts; no operator-side
opt-in is required. Create a client via the standard download-client
API:

```bash
curl -X POST -H 'Content-Type: application/json' \
     -d '{
       "id": "sab-main",
       "name": "Main SAB",
       "kind": "sabnzbd",
       "protocol": "usenet",
       "enabled": true,
       "host": "sab.lan",
       "port": 8080,
       "tls": false,
       "config": {
         "base_path": "/",
         "apikey": "0123456789abcdef0123456789abcdef"
       }
     }' \
     http://localhost:7878/api/v1/download-clients/
```

The `config` blob is documented in
[`api/openapi/loom.yaml`](../api/openapi/loom.yaml) as the
`SabnzbdConfig` schema. Recognised fields:

| Field       | Required | Default | Notes                                                          |
| ----------- | -------- | ------- | -------------------------------------------------------------- |
| `host`      | yes      | —       | Hostname or IP of SAB. Falls back to the top-level `host`.     |
| `port`      | no       | —       | TCP port. Falls back to the top-level `port`.                  |
| `tls`       | no       | `false` | Set when SAB is served over HTTPS.                             |
| `base_path` | no       | `/`     | Reverse-proxy subpath. Leading `/` required.                   |
| `apikey`    | yes      | —       | The SAB API key. Stored at rest, never returned on read.       |

After creating the client, probe it:

```bash
curl -X POST http://localhost:7878/api/v1/download-clients/sab-main/test
```

A `200 OK` with `status: "ok"` means Loom reached SAB and the apikey
was accepted.

## Category mapping

SAB's category model is the source of truth. Loom never creates,
edits, or deletes SAB categories — operators configure them in SAB's
UI under **Config → Categories**. Loom reads them in two ways:

1. **Rich form** (preferred). `mode=get_config&section=categories`
   returns `name` and `dir` per category; Loom surfaces both in
   `GET /api/v1/download-clients/{id}/categories`.
2. **Flat fallback.** Some SAB deployments lock the rich endpoint
   behind "advanced" config. Loom falls back to `mode=get_cats`,
   which returns names only.

When you submit an item, set `category` to a name SAB knows about.
SAB then applies its configured save path, post-processing level
(`pp`), and script for that category. The `*` "default" sentinel is
filtered out — Loom never advertises it as a real category.

## Submitting a download

```bash
curl -X POST -H 'Content-Type: application/json' \
     -d '{
       "url": "https://nzb.example/movie.nzb",
       "category": "movies",
       "name": "Some Movie 2024"
     }' \
     http://localhost:7878/api/v1/download-clients/sab-main/items
```

Loom honours the SAB-specific knobs via `tags`:

- `priority=1` — SAB priority (-2 paused, -1 low, 0 normal, 1 high,
  2 force).
- `script=postproc.sh` — name of a SAB user script to run after the
  job completes. The script must already exist in SAB.
- `pp=3` — SAB post-processing level (`0` skip, `1` repair, `2`
  unpack, `3` delete).

You can also POST raw NZB bytes (multipart upload) if you have the
file in hand instead of a fetchable URL.

## Lifecycle

| Action  | SAB endpoint                                                                |
| ------- | --------------------------------------------------------------------------- |
| Pause   | `mode=queue&name=pause&value=<nzo_id>` (empty value pauses the whole queue) |
| Resume  | `mode=queue&name=resume&value=<nzo_id>`                                     |
| Remove  | `mode=queue&name=delete` first, fall back to `mode=history&name=delete`     |

`deleteFiles=true` on remove maps to SAB's `del_files=1` on the
history endpoint; queue deletes always remove SAB's partial buffers
because there is nothing else to keep.

## Common gotchas

- **HTTPS reverse proxies and `base_path`.** If SAB lives at
  `https://media.example/sabnzbd/`, set `base_path: "/sabnzbd"` (no
  trailing slash). Loom appends `/api?...` itself; a missing or
  doubled slash is the most common 404 cause.
- **"API Key Incorrect".** SAB returns this with HTTP 200 and the
  envelope `{"status":false,"error":"API Key Incorrect"}`. Loom
  surfaces it as a typed `ErrAuth`. Re-check the key under SAB's
  Config → General → Security; rotation is a single click.
- **"Scripts not running".** SAB will only run a per-job script if
  the category has scripts enabled. Setting `script=` on the submit
  request without configuring it in SAB silently drops the value.
- **HTTP 401 / 403.** This indicates a reverse proxy or SAB itself
  rejected the request before it reached the apikey check. Check
  the proxy's auth headers and SAB's "Host whitelist" setting.
- **Self-signed certs.** Loom uses the system trust store; install
  your CA root or terminate TLS at a reverse proxy with a real
  cert.

## Health checking

The standard `downloads.health` job (see
[downloads.md](downloads.md)) covers SABnzbd. Health reports surface
the most recent test outcome plus the last `FreeSpace` reading from
SAB's incomplete-jobs directory.
