# Loom

> Unified media automation — **Radarr + Sonarr + Prowlarr** in one
> modern, container-native, observable platform. Single Go binary,
> SQLite by default, Postgres-ready, K8s-native.

[![CI](https://github.com/ebenderooock/loom/actions/workflows/ci.yml/badge.svg)](https://github.com/ebenderooock/loom/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/ebenderooock/loom)](https://goreportcard.com/report/github.com/ebenderooock/loom)
[![Release](https://img.shields.io/github/v/release/ebenderooock/loom?include_prereleases&sort=semver)](https://github.com/ebenderooock/loom/releases)
[![License: AGPL v3](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)
[![status: alpha](https://img.shields.io/badge/status-alpha-yellow)](ROADMAP.md)

> **Status: alpha.** The first tagged release ([`v0.1.0`](https://github.com/ebenderooock/loom/releases))
> is out — published binaries and container images are available. APIs and
> schema may still change. See the [roadmap](ROADMAP.md) for what's done,
> what's in flight, and what's next. Issues and PRs welcome.

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
| Storage | SQLite (default) + Postgres, embedded goose migrations (60+), sqlc-typed queries | Backup / restore CLI (Phase 11) |
| Health probes | `/healthz`, `/livez`, `/readyz` | — |
| Metrics | Prometheus `/metrics` (Go runtime + process collectors) | Loom-curated Grafana dashboards (Phase 11) |
| Tracing | OpenTelemetry SDK + OTLP/HTTP exporter | Spans on every module call as modules land |
| Profiling | `/debug/pprof/*`, gated by config | — |
| Logging | `slog` JSON, PII-redacted | Per-module loggers (rolling) |
| Config | Layered Viper (defaults → file → env → flags), hot-reload for safe keys | Live config UI (Phase 9) |
| Auth | Server-authoritative session cookies (argon2id) + API keys | OIDC + reverse-proxy header trust |
| Frontend | React 19 + TS + Vite + TanStack Router/Query + shadcn/ui + Tailwind; **Movies, Series, Indexers, Search, Downloads, Settings, Notifications pages** | Calendar, Import Lists (Phases 8–11) |

## Appearance

Admins can customize **Settings > Appearance** with a Light, Dark, AMOLED,
or System theme and a coordinated app palette. Start from Loom, Ocean, Ember,
or Forest, then use color pickers or hex values for primary, secondary, accent,
and surface tint. The palette drives buttons, navigation, pages, sidebars,
cards, dialogs, and focus rings; text contrast adjusts automatically. Surface
brightness follows the theme, while the tint controls its hue.
Individual color controls are tucked under **Customize individual colors**.
Invalid hex values stay visible for correction and must be fixed before saving.
Preview your choices, then save them for everyone or discard unsaved changes.

Backgrounds can use an image URL or an uploaded PNG, JPEG, or WebP (up to 5 MiB),
with a preview and adjustable dimming. Uploaded images persist in Loom and do
not require external hosting. Save applies these settings to everyone on the Loom
instance, including other devices; open browsers refresh them within a minute
or when the tab regains focus. System follows each device's OS preference.
Reset to defaults, then save, restores the standard Loom appearance.

External background URLs load directly in each browser, not through the Loom server.
Use a trusted HTTP(S) image URL reachable from every device (HTTPS when Loom
uses HTTPS); leave the URL empty to remove it. Items per page and default view
remain browser-local preferences.
These browsing preferences save automatically, separately from global appearance.

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

> Alpha — the server boots and serves health, metrics, and the
> `/api/v1/*` endpoints (movies, series, indexers, search, downloads,
> settings). The end-to-end search → grab → download → import pipeline
> works today.

### Docker (single container)

```bash
docker run --rm -p 1925:1925 \
  -v /opt/loom/config:/config \
  -v /mnt/media:/media \
  ghcr.io/ebenderooock/loom:latest
```

Open <http://localhost:1925/healthz>. Images are published to
`ghcr.io/ebenderooock/loom` (`:latest` and per-release tags such as
`:0.1.0`) on every tagged release.

### From source (works today)

Requires Go 1.26+ and Make. This path produces a working binary
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
curl -s http://localhost:1925/api/v1/system/status
# {"buildDate":"...","commit":"...","engine":"sqlite","version":"..."}
```

### Docker Compose (with Prometheus + Grafana + qBittorrent)

```bash
git clone https://github.com/ebenderooock/loom.git
cd loom
docker compose up -d
# Loom        http://localhost:1925
# qBittorrent http://localhost:8080
# Prometheus  http://localhost:9090
# Grafana     http://localhost:3000  (admin/admin)
```

See the [installation pathways](docs/deployment.md#installation-pathways) for
[Windows services](docs/windows.md), [Linux/systemd](docs/linux.md),
[macOS/launchd](docs/macos.md), [Raspberry Pi](docs/raspberry-pi.md),
[Docker](docs/docker.md), [Kubernetes/Helm](docs/kubernetes.md),
[Flux](docs/flux.md), and [Synology DSM](docs/synology.md).
The updated release pipeline bundles native installer scripts with the binaries;
older releases require scripts from this checkout, and Windows requires a
service-aware binary. The Synology guide covers
Container Manager and building a `.spk` for manual upload; SynoCommunity
availability depends on upstream publication.

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
- [Grow your library](docs/discovery.md)
- [Collections](docs/collections.md)
- [Security](docs/security.md)
- [Development setup](docs/development.md)
- [Loom TV — experimental native Android TV companion home](android-tv/README.md)
- [TV device pairing and server deployment](docs/device-pairing.md)
- [Contributing & code style](docs/contributing-style.md)
- [Architecture decision records](docs/adr/)

## Roadmap

The 11-phase delivery plan lives in [ROADMAP.md](ROADMAP.md). The core
media-automation pipeline (search → grab → download → import/organize for
movies and TV) is working and shipped in `v0.1.0`; wire-compatibility
APIs, request portal, and the migration importer are in progress.

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
