# Download clients

Loom Phase 3a introduces a pluggable download-client abstraction that
mirrors the indexer subsystem. Every download client — qBittorrent,
Transmission, Deluge, SABnzbd, NZBGet, the built-in `builtin/torrent`
in-process engine, and the built-in `builtin/null` no-op driver — is
registered into a single in-process registry, persists config and
last-known health to the database, and is reachable through the HTTP API
at `/api/v1/download-clients`.

This document is the operator-facing guide. For the design rationale and
the deviations from the original phase brief, see
[ADR-0014](adr/0014-download-clients-abstraction.md).

## API surface

| Method | Path                                                    | Description                              |
| ------ | ------------------------------------------------------- | ---------------------------------------- |
| GET    | `/api/v1/download-clients/`                             | List all clients with last-known health. |
| POST   | `/api/v1/download-clients/`                             | Create a download client.                |
| GET    | `/api/v1/download-clients/{id}`                         | Fetch one client + health.               |
| PUT    | `/api/v1/download-clients/{id}`                         | Replace a client.                        |
| PATCH  | `/api/v1/download-clients/{id}`                         | Update a subset of fields.               |
| DELETE | `/api/v1/download-clients/{id}`                         | Delete a client.                         |
| POST   | `/api/v1/download-clients/{id}/test`                    | Probe and persist health.                |
| GET    | `/api/v1/download-clients/{id}/categories`              | List categories advertised.              |
| GET    | `/api/v1/download-clients/{id}/free-space`              | Bytes free; `-1` if unknown.             |
| GET    | `/api/v1/download-clients/{id}/items`                   | List active items (`?ids=a,b` to limit). |
| POST   | `/api/v1/download-clients/{id}/items`                   | Submit a magnet/torrent/NZB.             |
| POST   | `/api/v1/download-clients/{id}/pause`                   | Pause items (empty body = all).          |
| POST   | `/api/v1/download-clients/{id}/resume`                  | Resume items (empty body = all).         |

The full schema is in [`api/openapi/loom.yaml`](../api/openapi/loom.yaml)
under the `download-clients` tag.

## Configuration

Download-client behaviour is governed by a top-level `downloads:` block
in the Loom config:

```yaml
downloads:
  operation_timeout: 15        # seconds; per-client request timeout
  max_parallel: 8              # ceiling on concurrent fan-out probes
  health_check_schedule: "*/5 * * * *"
  health_check_timeout: 10     # seconds; per-client health probe timeout
```

All four keys have sane defaults. The health check job is registered as
`downloads.health` and runs through Loom's standard cron scheduler.

## Built-in `null` client

The `builtin/null` kind is always registered. It accepts any input,
reports `protocol: torrent`, and returns empty `Status`/`Categories`
results plus `FreeSpace = -1`. It exists so that fresh installs and
test environments can exercise CRUD and health endpoints without
configuring a real client. Most operators will never need it in
production.

```bash
curl -sS -H "X-Api-Key: $LOOM_API_KEY" \
     -H "Content-Type: application/json" \
     -d '{"id":"null","name":"Null","kind":"builtin/null","protocol":"torrent","enabled":true}' \
     http://localhost:7878/api/v1/download-clients/
```

## Built-in torrent client (`builtin/torrent`)

`builtin/torrent` is an in-process BitTorrent engine backed by
[anacrolix/torrent](https://github.com/anacrolix/torrent). It requires
**no external process** — everything runs inside the Loom binary.

### Feature parity

| Feature                        | Supported |
| ------------------------------ | --------- |
| Magnet URIs                    | ✅        |
| Raw `.torrent` bytes           | ⏳ planned |
| Torrent URL (HTTP)             | ⏳ planned |
| Pause / Resume (per item)      | ✅        |
| Pause / Resume all             | ✅        |
| Remove                         | ✅        |
| Data verify (recheck)          | ✅        |
| DHT re-announce                | ✅        |
| Global download speed limit    | ✅        |
| Global upload speed limit      | ✅        |
| Per-item speed limits          | ⏳ planned |
| Queue priority (top/bottom)    | ⏳ planned |
| Force-start                    | ⏳ planned |
| Seeding lifecycle detection    | ✅        |
| `FreeSpace` reporting          | ✅        |
| Per-item detail (peers/files)  | ⏳ planned |

### Configuration

The `config` JSON blob accepts the following keys:

| Key                    | Type   | Default | Description                                          |
| ---------------------- | ------ | ------- | ---------------------------------------------------- |
| `download_dir`         | string | **required** | Where downloaded files are written.             |
| `listen_port`          | int    | `6881`  | TCP/UDP port for incoming peer connections.          |
| `enable_dht`           | bool   | `true`  | Enable Distributed Hash Table peer discovery.        |
| `enable_pex`           | bool   | `true`  | Enable Peer EXchange extensions.                     |
| `enable_upnp`          | bool   | `true`  | Enable UPnP/NAT-PMP port mapping.                    |
| `download_speed_limit` | int64  | `0`     | Bytes per second; `0` = unlimited.                   |
| `upload_speed_limit`   | int64  | `0`     | Bytes per second; `0` = unlimited.                   |

### Example: create via API

```bash
curl -sS -X POST \
     -H "X-Api-Key: $LOOM_API_KEY" \
     -H "Content-Type: application/json" \
     -d '{
           "kind": "builtin/torrent",
           "name": "Built-in Torrent",
           "protocol": "torrent",
           "enabled": true,
           "config": {
             "download_dir": "/data/downloads",
             "listen_port": 6881,
             "enable_dht": true,
             "enable_pex": true,
             "enable_upnp": true,
             "download_speed_limit": 0,
             "upload_speed_limit": 0
           }
         }' \
     http://localhost:7878/api/v1/download-clients/
```

### Engine status endpoint

Once a `builtin/torrent` client is created you can query the live engine
summary:

```bash
GET /api/v1/download-clients/{id}/torrent/status
```

Returns aggregate counts (downloading, seeding, paused, queued) plus the
current aggregate transfer rates and configured speed limits.

### Speed limit endpoint

```bash
POST /api/v1/download-clients/{id}/torrent/speed-limits
Content-Type: application/json

{ "download_limit": 10485760, "upload_limit": 5242880 }
```

Sets rates immediately and persists them to the stored config so they
survive restarts. Pass `0` for unlimited.

### Pause/resume all torrents

```bash
POST /api/v1/download-clients/{id}/torrent/pause-all
POST /api/v1/download-clients/{id}/torrent/resume-all
```

### Implementation notes

- **Pause semantics**: anacrolix/torrent does not have a first-class
  "pause" concept. Loom implements pause by calling
  `DisallowDataDownload()` and tracking the paused state internally.
  `Resume` calls `AllowDataDownload()` and clears the paused flag.
- **Rate limiting**: global rate limiters are created once at startup and
  mutated via `rate.Limiter.SetLimit()` — changes take effect
  immediately without restarting the engine.
- **Build note**: the project must be built with `CGO_ENABLED=0` due to a
  pre-existing duplicate sqlite symbol conflict in the dependency tree.
  This is unrelated to the torrent engine and affects the main branch too.

## Health checking

Each `POST /test` call probes the client (`Test()`), and on success
opportunistically calls `Categories()` and `FreeSpace()` to enrich the
persisted health row. The scheduled `downloads.health` job runs the
same probe in the background for every enabled client, capping
parallelism at `downloads.max_parallel`. Health rows include:

- `status` — `unknown`, `ok`, `warning`, or `error`
- `last_checked_at`
- `last_error`
- `consecutive_failures`
- `last_free_space_bytes` (omitted when the client cannot report it)

## Authoring a new download-client driver

1. Create a package under `internal/downloads/<your-kind>/` (or under
   `internal/downloads/builtin/` for in-tree built-ins).
2. Implement [`downloads.DownloadClient`](../internal/downloads/types.go).
3. In an `init()` function, register a factory:

   ```go
   func init() {
       downloads.RegisterKind(KindFoo, factoryFn)
   }
   ```

4. Add the package to a blank import list (the same pattern indexers
   use) so `init()` runs.

The registry/service/health/HTTP layers do not change. Persistence is
handled by `internal/downloads/repository.go` for both SQLite and
Postgres via sqlc-generated row types.

## Storage

Migration `0010` adds two tables on both engines:

- `download_clients` — config, defaults, transport metadata.
- `download_client_health` — last probe outcome, with cached
  categories JSON and free-space bytes.

Credentials in the `config` JSON blob are stored at rest in plaintext,
matching the existing indexers/proxies convention. ADR-0014 captures
the rationale and the future plan to add at-rest encryption uniformly
across the three subsystems.

## SABnzbd

The `sabnzbd` kind ships in-tree as the first Usenet driver on this
abstraction. It speaks the SABnzbd 3.x+ JSON API, authenticates with
the operator's apikey, and maps SAB's queue and history vocabularies
onto Loom's `ItemStatus` enum. See
[`docs/downloads-sabnzbd.md`](downloads-sabnzbd.md) and
[ADR-0016](adr/0016-sabnzbd-download-client-kind.md) for the operator
guide and design rationale.

## Supported kinds

| Kind                  | Protocol | Status        | Docs                                                       |
| --------------------- | -------- | ------------- | ---------------------------------------------------------- |
| `builtin/null`        | torrent  | ✅ shipped    | (in-tree no-op; see [ADR-0014](adr/0014-download-clients-abstraction.md)) |
| `builtin/torrent`     | torrent  | ✅ shipped    | [Built-in torrent client](#built-in-torrent-client-builtintorrent) |
| `qbittorrent`         | torrent  | ✅ shipped    | [docs/downloads-qbittorrent.md](downloads-qbittorrent.md) |
| `transmission`        | torrent  | ⏳ planned    |                                                            |
| `deluge`              | torrent  | ⏳ planned    |                                                            |
| `sabnzbd`             | usenet   | ⏳ planned    |                                                            |
| `nzbget`              | usenet   | ✅ shipped    | [docs/downloads-nzbget.md](downloads-nzbget.md)            |

## Routing & Monitoring (Phase 3g)

Loom Phase 3g introduces the download routing and monitoring layer, which bridges the indexer intake pipeline and download clients.

### Router Service

The `Router` service subscribes to indexer result events and automatically queues high-quality results on configured download clients. It implements a simple quality filter (seeder-based) and attempts clients in priority order, falling back to any available client on failure.

**Quality filtering (Phase 3):**
- Reject torrents with 0 seeders
- Accept Usenet results (no seeders field)
- Accept torrents with >0 seeders

Full semantic filtering (resolution, codec, release groups, language) is deferred to Phase 5.

**Event emission:**
- `TopicDownloadQueued` — result successfully queued on a client
- `TopicDownloadFailed` — all clients unavailable or all failed to queue
- Events include origin result ID, client ID, download ID, and timestamp for traceability

### Monitor Service

The `Monitor` service periodically polls all registered download clients for status updates and emits completion events. It tracks completed items to avoid duplicate event emission across sweeps.

**Periodic invocation:** The Monitor is designed to be called by the scheduler (kernel phase integration pending). Each `Run()` call fans out Status queries across all clients and emits `TopicDownloadCompleted` events for newly-completed items.

**Event emission:**
- `TopicDownloadCompleted` — item transitioned to completed status in this sweep (not previously seen as completed)

### Event Topics

The download orchestration layer defines three new event bus topics:

- `downloads.queued` — result successfully routed to a download client
- `downloads.failed` — result failed to route (no clients or all clients failed)
- `downloads.completed` — download item completed

See [ADR-0020](adr/0020-download-routing-and-monitoring.md) for the full design rationale.

