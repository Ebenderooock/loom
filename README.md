# Loom

> Unified media automation — Radarr + Sonarr + Prowlarr in one modern,
> container-native, observable platform.

[![License: AGPL v3](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)
[![status: pre-alpha](https://img.shields.io/badge/status-pre--alpha-orange)]()

Loom is a from-scratch, like-for-like replacement of the *arr stack
(Radarr, Sonarr, Prowlarr) with a modernized UI, first-class observability,
and a single binary that runs anywhere. It is **wire-compatible** with the
existing self-hosted media ecosystem, so Overseerr, Jellyseerr, Bazarr,
Notifiarr, Tautulli, Plex, Jellyfin and Emby keep working unchanged.

## Status

**Pre-alpha.** Active development. Not ready for production. Tracking issues
welcome; PRs once Phase 1 lands.

## Why Loom?

| | *arr stack | Loom |
|---|---|---|
| Apps to deploy | 3+ (Radarr, Sonarr, Prowlarr, …) | **1** (modular monolith; split-mode optional) |
| Databases | 3 SQLite files | 1 SQLite (or Postgres) |
| Runtime | .NET (heavier images) | Go static binary (distroless, ~30 MB image) |
| Observability | Ad-hoc logs | OpenTelemetry, Prometheus, structured logs, pprof from day one |
| Customization | Forks | First-class plugin SDK (gRPC, out-of-process) |
| UI | Three different UIs | One unified, mobile-first, accessible UI |
| Migration | n/a | Built-in importer from existing arr SQLite DBs |

## Compatibility

Loom exposes wire-compatible API surfaces so existing clients keep working:

- `/api/v3/*` — Radarr v3 shape (Overseerr, Jellyseerr, Notifiarr)
- `/api/v3/*` and v4 — Sonarr (Overseerr, Bazarr)
- `/api/v1/*` — Prowlarr (used by Sonarr/Radarr-as-clients)
- Cardigann YAML indexer definitions (Jackett/Prowlarr ecosystem)
- Newznab / Torznab (inbound and outbound)
- Download clients: qBittorrent, Transmission, Deluge, rTorrent, SABnzbd, NZBGet

## Quickstart (preview, post-Phase-1)

```bash
docker run --rm -p 8989:8989 \
  -v /path/to/config:/config \
  -v /path/to/media:/media \
  ghcr.io/loomctl/loom:latest
```

Open <http://localhost:8989>.

## Architecture

See [docs/adr/](docs/adr/) for architecture decisions and the
[plan](.github/PLAN.md) (mirrored on the project site) for the phased roadmap.

```
┌─────────────────────────────────────────────────────────┐
│                       loom (binary)                     │
│  API Gateway (REST/WS, /api/v1 + compat surfaces)       │
│  ── Internal Event Bus ────────────────────────────     │
│  Indexers │ Movies │ Series │ Downloads │ Metadata │ …  │
│  Shared kernel: scheduler, config, parser, importer,    │
│                 notifications, history, queue, RSS      │
│  Storage: SQLite (default) or Postgres                  │
└─────────────────────────────────────────────────────────┘
```

## Development

Requirements: Go 1.23+, Node 20+, Docker.

```bash
# build everything
make build

# run with hot reload
make dev

# run tests
make test
```

## Migrating from Radarr / Sonarr / Prowlarr

```bash
loom migrate import --from radarr   --db /path/to/radarr.db   --dry-run
loom migrate import --from sonarr   --db /path/to/sonarr.db   --dry-run
loom migrate import --from prowlarr --db /path/to/prowlarr.db --dry-run
```

Drop `--dry-run` once the diff looks right. Re-runs are idempotent.

## License

[AGPL-3.0](LICENSE). The same license family as the upstream *arr stack.

## Acknowledgements

Loom is **not** a fork. It is a clean-room reimplementation that honors the
years of community work in Radarr, Sonarr, Prowlarr, Jackett, Cardigann,
TRaSH-Guides and the broader self-hosted media community. Thank you.
