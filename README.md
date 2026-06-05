# Loom

> Unified media automation — **Radarr + Sonarr + Prowlarr** in one
> modern, container-native, observable platform. Single Go binary,
> SQLite by default, Postgres-ready, K8s-native.

[![CI](https://img.shields.io/badge/ci-pending-lightgrey)](.github/workflows/ci.yml)
[![Go Report](https://img.shields.io/badge/go%20report-pending-lightgrey)](https://goreportcard.com/)
[![License: AGPL v3](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)
[![Discord](https://img.shields.io/badge/discord-pending-lightgrey)](https://discord.gg/)
[![status: pre-alpha](https://img.shields.io/badge/status-pre--alpha-orange)](ROADMAP.md)

> **Status: pre-alpha.** Active development. Not ready for production.
> See the [roadmap](ROADMAP.md) for what's done, what's in flight, and
> what's coming. Tracking issues welcome; PRs once Phase 1 lands.

Loom is a from-scratch, like-for-like replacement of the *arr stack
(Radarr, Sonarr, Prowlarr) with a modernised UI, first-class
observability, and a single binary that runs anywhere. It is
**wire-compatible** with the existing self-hosted media ecosystem, so
Overseerr, Jellyseerr, Bazarr, Notifiarr, Tautulli, Plex, Jellyfin and
Emby keep working unchanged.

> **Name rationale — "Loom":** short, catchy, evokes weaving disparate
> threads (indexers, download clients, metadata providers, libraries)
> into a uniform fabric. Working name; trivial to rename later.

## Why Loom?

| | *arr stack | Loom |
|---|---|---|
| Apps to deploy | 3+ (Radarr, Sonarr, Prowlarr, …) | **1** (modular monolith; split-mode optional) |
| Databases | 3 SQLite files | 1 SQLite (or Postgres) |
| Runtime | .NET (heavier images) | Go static binary (distroless, ~30 MB image) |
| Observability | Ad-hoc logs | OpenTelemetry, Prometheus, structured JSON logs, pprof from day one |
| Customisation | Forks | First-class plugin SDK (gRPC, out-of-process — Phase 10) |
| UI | Three different UIs | One unified, mobile-first, accessible UI |
| Migration | n/a | Built-in importer from existing arr SQLite DBs (Phase 8) |

## Feature highlights

| Surface | Today (Phase 1) | Coming |
|---|---|---|
| HTTP server | chi router, request-ID, structured access logs, panic recovery, gzip, ETag, CORS | WebSocket events (Phase 1c) |
| Storage | SQLite (default) + Postgres, embedded goose migrations (43), sqlc-typed queries | Backup / restore CLI (Phase 11) |
| Health probes | `/healthz`, `/livez`, `/readyz` | — |
| Metrics | Prometheus `/metrics` (Go runtime + process collectors) | Loom-curated Grafana dashboards (Phase 11) |
| Tracing | OpenTelemetry SDK + OTLP/HTTP exporter | Spans on every module call as modules land |
| Profiling | `/debug/pprof/*`, gated by config | — |
| Logging | `slog` JSON, PII-redacted | Per-module loggers (rolling) |
| Config | Layered Viper (defaults → file → env → flags), hot-reload for safe keys | Live config UI (Phase 9) |
| Auth | Server-authoritative session cookies (argon2id) + API keys | OIDC + reverse-proxy header trust |
| Frontend | React 19 + TS + Vite + TanStack Router/Query + shadcn/ui + Tailwind; **Movies, Series, Indexers, Search, Downloads, Settings, Notifications pages** | Calendar, Import Lists (Phases 8–11) |

## Wire-compatibility (Phase 7)

The following surfaces will be wire-compatible so existing clients keep
working:

- `/api/v3/*` — Radarr v3 (Overseerr, Jellyseerr, Notifiarr).
- `/api/v3/*` and v4 — Sonarr (Overseerr, Bazarr).
- `/api/v1/*` — Prowlarr v1 (used by Sonarr/Radarr-as-clients).
- Cardigann YAML indexer definitions (Jackett / Prowlarr ecosystem) —
  see [`docs/indexers-cardigann.md`](docs/indexers-cardigann.md).
- Newznab / Torznab (inbound and outbound).
- Download clients: qBittorrent, Transmission, Deluge, rTorrent, SABnzbd, NZBGet.

## Quickstart

> Pre-alpha — these flows boot the server and serve the health, metrics,
> and `/api/v1/system/status` endpoints. Library/indexer/download
> functionality lands in later phases.

### Docker (single container, post-Phase-1 — image not yet published)

```bash
docker run --rm -p 8989:8989 \
  -v /opt/loom/config:/config \
  -v /mnt/media:/media \
  ghcr.io/ebenderooock/loom:latest
```

Open <http://localhost:8989/healthz>. The image lands publicly with
the first tagged release (Phase 11). Until then, prefer the from-source
path below.

### From source (works today)

Requires Go 1.23+ and Make. This path produces a working binary
against the current `master`.

```bash
git clone https://github.com/ebenderooock/loom.git
cd loom
make build
mkdir -p ./run
LOOM_CONFIG_DIR=./run \
LOOM_DATA_DIR=./run \
LOOM_STORAGE_SQLITE_PATH=./run/loom.db \
  ./dist/loom serve
# in another terminal:
curl -s http://localhost:8989/api/v1/system/status
# {"buildDate":"...","commit":"...","engine":"sqlite","version":"..."}
```

### Docker Compose (with Prometheus + Grafana + qBittorrent)

```bash
git clone https://github.com/ebenderooock/loom.git
cd loom
docker compose up -d
# Loom        http://localhost:8989
# qBittorrent http://localhost:8080
# Prometheus  http://localhost:9090
# Grafana     http://localhost:3000  (admin/admin)
```

See [`docs/deployment.md`](docs/deployment.md) for Kubernetes
(Phase 11), bare-metal binaries, and reverse-proxy snippets
(Traefik / Caddy / Nginx). For Synology NAS, see
[`docs/synology.md`](docs/synology.md).

## Documentation

The [`docs/`](docs/) directory is the source of truth.

- [Configuration reference](docs/configuration.md)
- [Architecture](docs/architecture.md)
- [Deployment](docs/deployment.md)
- [Synology (DSM) install](docs/synology.md)
- [Observability](docs/observability.md)
- [Storage](docs/storage.md)
- [API](docs/api.md) (and [`api/openapi/loom.yaml`](api/openapi/loom.yaml))
- [Indexers — aggregator (Newznab/Torznab)](docs/indexers-aggregator.md)
- [Indexers — rate limits and retry/backoff](docs/indexers-rate-limits.md)
- [Download clients](docs/downloads.md)
- [Security](docs/security.md)
- [Development setup](docs/development.md)
- [Contributing & code style](docs/contributing-style.md)
- [Architecture decision records](docs/adr/)

## Roadmap

The 11-phase delivery plan lives in [ROADMAP.md](ROADMAP.md). Phases 0–2
are complete; Phases 3–7 are substantially done; Phase 3 (import pipeline)
is the current focus.

## Contributing

PRs welcome — please read [`CONTRIBUTING.md`](CONTRIBUTING.md) and
[`docs/contributing-style.md`](docs/contributing-style.md) first.

> **No upstream code copied from Radarr / Sonarr / Prowlarr.** Loom is a
> clean-room reimplementation that references public API specs, docs,
> and user-facing config files only.

For security issues see [`SECURITY.md`](SECURITY.md).

## License

[AGPL-3.0](LICENSE). Same license family as the upstream *arr stack.

## Acknowledgements

Loom is **not** a fork. It is a clean-room reimplementation that honours
the years of community work in Radarr, Sonarr, Prowlarr, Jackett,
Cardigann, TRaSH-Guides and the broader self-hosted media community.
Thank you.
