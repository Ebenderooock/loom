# NZBGet download client

> Configure Loom to hand NZBs off to a running NZBGet 21 or newer
> instance.

This page is the operator's guide to the `nzbget` download client
kind. For the design rationale, deferred features, and the full
state-mapping table see
[ADR-0019](adr/0019-nzbget-download-client-kind.md).

## What you need before you start

- A running NZBGet **21.0 or newer**. Older builds are likely to
  work for everything except `Add()` edge cases (post-process
  scripts), but are not supported.
- The NZBGet **ControlUsername** and **ControlPassword**. Both are
  configured in NZBGet under **Settings → Security**. NZBGet has
  no API key concept — it authenticates with HTTP Basic Auth only.
- Network reachability from Loom to NZBGet. If NZBGet sits behind a
  reverse proxy at a subpath (e.g.
  `https://media.example/nzbget/`), record the prefix — you will
  pass it as `base_path`.
- **`ControlIP` set to a value Loom can reach.** Out of the box
  NZBGet listens on `0.0.0.0`. If you have hardened it to
  `127.0.0.1`, Loom on a different host will be locked out — see
  [Common gotchas](#common-gotchas).

## Configuration

The NZBGet client is registered when Loom starts; no operator-side
opt-in is required. Create a client via the standard download-client
API:

```bash
curl -X POST -H 'Content-Type: application/json' \
     -d '{
       "id": "nzbget-main",
       "name": "Main NZBGet",
       "kind": "nzbget",
       "protocol": "usenet",
       "enabled": true,
       "host": "nzbget.lan",
       "port": 6789,
       "tls": false,
       "config": {
         "base_path": "/",
         "username": "loom",
         "password": "hunter2"
       }
     }' \
     http://localhost:7878/api/v1/download-clients/
```

The `config` blob is documented in
[`api/openapi/loom.yaml`](../api/openapi/loom.yaml) as the
`NzbgetConfig` schema. Recognised fields:

| Field       | Required | Default | Notes                                                          |
| ----------- | -------- | ------- | -------------------------------------------------------------- |
| `host`      | yes      | —       | Hostname or IP of NZBGet. Falls back to the top-level `host`.  |
| `port`      | no       | —       | TCP port (typically `6789`). Falls back to the top-level `port`.|
| `tls`       | no       | `false` | Set when NZBGet is fronted by HTTPS.                           |
| `base_path` | no       | `/`     | Reverse-proxy subpath. Leading `/` required.                   |
| `username`  | yes      | —       | NZBGet `ControlUsername`.                                      |
| `password`  | yes      | —       | NZBGet `ControlPassword`. Stored at rest, never returned.      |

After creating the client, probe it:

```bash
curl -X POST http://localhost:7878/api/v1/download-clients/nzbget-main/test
```

A `200 OK` with `status: "ok"` means Loom reached the JSON-RPC
endpoint at `<base_path>/jsonrpc` and the credentials were
accepted.

## Category mapping

NZBGet's category model is the source of truth. Loom never
creates, edits, or deletes NZBGet categories — operators configure
them in NZBGet under **Settings → Categories**. Loom reads them by
issuing the `config()` RPC and walking the keyspace for
`Category{N}.Name` / `Category{N}.DestDir` pairs; the result is
cached in-process for 30 seconds so a hot path that calls
`Categories()` per add does not hit the RPC every time.

Empty slots between numbered categories are skipped — NZBGet
preserves gaps when you delete a category from the middle of the
list, and Loom does not surface them.

When you submit an item, set `category` to a name NZBGet knows
about. NZBGet then applies its configured `DestDir`,
post-processing parameters, and scripts for that category.

## Submitting a download

```bash
curl -X POST -H 'Content-Type: application/json' \
     -d '{
       "url": "https://nzb.example/movie.nzb",
       "category": "movies",
       "name": "Some Movie 2024"
     }' \
     http://localhost:7878/api/v1/download-clients/nzbget-main/items
```

Both URL and raw-bytes submission are supported. URL adds use
NZBGet 17+'s server-side fetch (the URL is passed as
`NZBFilename` with an empty `NZBContent` payload). Raw-bytes adds
base64-encode the NZB content into `NZBContent`.

Loom honours the NZBGet-specific knobs via `tags`:

- `priority=50` — NZBGet priority. Conventional values: `-100`
  very low, `-50` low, `0` normal, `50` high, `100` very high,
  `900` force.
- `add_to_top` — push the item to the front of the queue rather
  than appending.
- `add_paused` — submit the item but leave it paused; resume later
  via the lifecycle API.
- `dupekey=<string>`, `dupescore=<int>`, `dupemode=<SCORE|ALL|FORCE>`
  — NZBGet's duplicate-detection knobs.
- `pp_<name>=<value>` — pass-through to NZBGet's per-job
  post-process parameters (e.g. `pp_passwd=hunter2` to set an
  archive password). The `pp_` prefix is stripped before sending.

## Lifecycle

| Action                       | NZBGet RPC                                                                |
| ---------------------------- | ------------------------------------------------------------------------- |
| Pause (no ids)               | `pausedownload`                                                           |
| Pause (specific ids)         | `editqueue("GroupPause", 0, "", IDList)`                                  |
| Resume (no ids)              | `resumedownload`                                                          |
| Resume (specific ids)        | `editqueue("GroupResume", 0, "", IDList)`                                 |
| Remove (`deleteFiles=false`) | `editqueue("GroupDelete", 0, "", IDList)` — preserves history + on-disk  |
| Remove (`deleteFiles=true`)  | `editqueue("GroupFinalDelete", 0, "", IDList)` — purges history + files  |

The split between `GroupDelete` and `GroupFinalDelete` is
deliberate: NZBGet itself separates the operations and Loom does
not silently combine them. An operator who explicitly asks for
`deleteFiles=false` is guaranteed history rows and on-disk bytes
survive.

## Common gotchas

- **HTTPS reverse proxies and `base_path`.** If NZBGet lives at
  `https://media.example/nzbget/`, set `base_path: "/nzbget"` (no
  trailing slash). Loom appends `/jsonrpc` itself; a missing or
  doubled slash is the most common 404 cause.
- **`ControlIP=127.0.0.1` locks out Loom.** A common hardening tip
  is to bind NZBGet to localhost. Loom on another host (or in a
  container with its own network namespace) cannot reach it. Use
  `ControlIP=0.0.0.0` and rely on a firewall, or co-locate Loom
  with NZBGet.
- **HTTP 401.** Loom surfaces this as `ErrAuth`. The most common
  causes are a stale `ControlPassword` after a NZBGet config
  reload, or a reverse proxy stripping the `Authorization` header.
- **Post-processing scripts run from the wrong working directory.**
  NZBGet runs scripts with `DestDir` as `cwd`, not the script's
  install directory. If a script breaks under Loom that worked
  manually, that is almost always the cause — fix the script, not
  Loom.
- **Sizes look small.** NZBGet reports sizes as **binary** MB
  (`FileSizeMB * 1024 * 1024` bytes). Loom converts; do not be
  surprised if a human-eye comparison against `du -h` differs by a
  few percent — that is the binary-vs-decimal MB delta.
- **Categories with empty `DestDir`.** NZBGet allows a category
  with no override path; jobs in such a category land in
  `MainDir`. Loom surfaces these categories with an empty
  `save_path`, which is correct.

## Health checking

The standard `downloads.health` job (see
[downloads.md](downloads.md)) covers NZBGet. Health reports
surface the most recent test outcome plus the last `FreeSpace`
reading from NZBGet's `MainDir`.
