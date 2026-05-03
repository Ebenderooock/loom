# Download clients

Loom Phase 3a introduces a pluggable download-client abstraction that
mirrors the indexer subsystem. Every download client — qBittorrent,
Transmission, Deluge, SABnzbd, NZBGet, and the built-in `builtin/null`
no-op driver — is registered into a single in-process registry, persists
config and last-known health to the database, and is reachable through
the HTTP API at `/api/v1/download-clients`.

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
| `qbittorrent`         | torrent  | ✅ shipped    | [docs/downloads-qbittorrent.md](downloads-qbittorrent.md) |
| `transmission`        | torrent  | ⏳ planned    |                                                            |
| `deluge`              | torrent  | ⏳ planned    |                                                            |
| `sabnzbd`             | usenet   | ⏳ planned    |                                                            |
| `nzbget`              | usenet   | ⏳ planned    |                                                            |
