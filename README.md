# Loom

> Unified media automation — **Radarr + Sonarr + Prowlarr** in one
> modern, container-native, observable platform. Single Go binary,
> SQLite by default, Postgres-ready, K8s-native.

[![CI](https://github.com/ebenderooock/loom/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/ebenderooock/loom/actions/workflows/ci.yml)
[![Main image](https://github.com/ebenderooock/loom/actions/workflows/publish-main.yml/badge.svg?branch=main)](https://github.com/ebenderooock/loom/actions/workflows/publish-main.yml)
[![Prerelease image](https://github.com/ebenderooock/loom/actions/workflows/publish-prerelease.yml/badge.svg)](https://github.com/ebenderooock/loom/actions/workflows/publish-prerelease.yml)
[![Release workflow](https://github.com/ebenderooock/loom/actions/workflows/release.yml/badge.svg)](https://github.com/ebenderooock/loom/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/ebenderooock/loom)](https://goreportcard.com/report/github.com/ebenderooock/loom)
[![Latest stable](https://img.shields.io/github/v/release/ebenderooock/loom?label=latest%20stable&sort=semver)](https://github.com/ebenderooock/loom/releases/latest)
[![Latest release including prereleases](https://img.shields.io/github/v/release/ebenderooock/loom?include_prereleases&label=latest%20release&sort=semver)](https://github.com/ebenderooock/loom/releases)
[![License: AGPL v3](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)
[![status: alpha](https://img.shields.io/badge/status-alpha-yellow)](ROADMAP.md)

> **Status: alpha.** [Tagged releases](https://github.com/ebenderooock/loom/releases)
> provide published binaries and container images. The badges above track
> CI, publishing status, and release versions. APIs and
> schema may still change. See the [roadmap](ROADMAP.md) for what's done,
> what's in flight, and what's next. Issues and PRs welcome.

Loom is a from-scratch media automation platform aiming to replace the *arr stack
(Radarr, Sonarr, Prowlarr) with a modernised UI, first-class
observability, and a single binary that runs anywhere. It is
designed to integrate with the existing self-hosted media ecosystem.
Selected Radarr/Sonarr/Prowlarr compatibility endpoints and media-server
integrations exist, but full API parity and unchanged behavior in every
downstream app are not guaranteed.

> **Name rationale — "Loom":** short, catchy, evokes weaving disparate
> threads (indexers, download clients, metadata providers, libraries)
> into a uniform fabric. Working name; trivial to rename later.

## Why Loom?

|                | *arr stack                       | Loom                                                                |
| -------------- | -------------------------------- | ------------------------------------------------------------------- |
| Apps to deploy | 3+ (Radarr, Sonarr, Prowlarr, …) | **1** (modular monolith; split-mode optional)                       |
| Databases      | 3 SQLite files                   | 1 SQLite (or Postgres)                                              |
| Runtime        | .NET (heavier images)            | Go static binary (distroless, ~30 MB image)                         |
| Observability  | Ad-hoc logs                      | OpenTelemetry, Prometheus, structured JSON logs, pprof from day one |
| Customisation  | Forks                            | Admin-defined JavaScript event plugins; external SDK/marketplace planned |
| UI             | Three different UIs              | One unified, mobile-first, accessible UI                            |
| Migration      | n/a                              | Limited importer from existing arr SQLite DBs; not a lossless cutover |

## Feature highlights

The [reviewed roadmap](ROADMAP.md#what-can-be-tested) distinguishes published
`v0.4.1` capabilities from later work on `main`. The summary below describes
the current source, not a promise that an older binary includes every feature.

| Surface | Current source |
| ------- | -------------- |
| Movies and TV | Libraries, metadata, monitoring, search/grab, download monitoring and imports |
| Discovery | Import lists, calendar, library-expansion suggestions and mixed movie/TV collections |
| Requests | Authenticated movie/series/artist requests, quotas, admin approval and Discord/Telegram bots |
| Music | Feature-gated MusicBrainz catalog and acquisition; important import/profile fixes postdate `v0.4.1` |
| Operations | SQLite or Postgres, health probes, Prometheus, OpenTelemetry, structured logs and SQLite/config backup CLI |
| Auth | Sessions, API keys, users/invites, OIDC and opt-in reverse-proxy trust |
| Experimental | Feature-gated Rain-sidecar torrent integration, media analytics and JavaScript event plugins |

## Title quick actions

Movie and series detail views have a **More actions** (`...`) menu with
**Sync with Plex**, **Sync with Jellyfin**, **Refresh metadata**, **Rescan library files**,
**View import history**, and **Copy folder path**.
History is filtered to the selected movie or the selected series's episodes.
The other actions require an administrator.

Plex and Jellyfin sync require complete registered media in the title's library
and an enabled, configured connection for the respective server; each action
is hidden without one. These actions request a **library-wide refresh** on each
enabled server of the selected type, not a title-specific scan, download, or
watched-state sync. Loom paths are not sent to either server, so different
Windows/Linux mount paths are safe.
Success means the server accepted the refresh, not that indexing has finished.

The file rescan **discovers new files in the title's library folder** and updates
availability, even when the title is marked missing or has no registered files.
It remains available when no folder is currently found; running it rechecks the
folder and reports an error if it is still unavailable.
Temporary files, symbolic links, shared/ambiguous folders, and
paths outside the title's assigned library are excluded. Copy uses the resolved
existing title folder, never a guessed folder name. Series metadata refresh
preserves episode IDs and file links rather than rebuilding the episode catalog
or invoking the file-ingestion hook. None of these quick actions enqueue
downloads or retry imports from download folders.

## Appearance

Admins can customize **Settings > Appearance** with a Light, Dark, AMOLED,
or System theme and a coordinated app palette. Start from Loom, Ocean, Ember,
or Forest, then use color pickers or hex values for primary, secondary, accent,
and surface tint. The palette drives buttons, navigation, pages, sidebars,
cards, dialogs, focus rings, and the Loom logo; text contrast adjusts
automatically. Surface
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

## Ecosystem integrations

Loom exposes selected compatibility endpoints under `/compat/radarr`,
`/compat/sonarr` and `/compat/prowlarr`. These are partial adapters, not full
replacements for the upstream APIs; some command and sync endpoints only
acknowledge requests. Test the exact downstream workflow you depend on.

Indexer integrations include Cardigann definitions and Newznab/Torznab
(inbound and outbound). External download clients include qBittorrent,
Transmission, Deluge, SABnzbd and NZBGet.

**Usenet/NNTP has not been tested at all.** The Newznab/NZB and external
SABnzbd/NZBGet implementations are unverified; a built-in Usenet downloader
is not implemented.

## Quickstart

> Alpha — the server boots and serves health, metrics, and the
> `/api/v1/*` endpoints (movies, series, indexers, search, downloads,
> settings). The search → grab → download → import pipeline is wired, but
> import reliability fixes on `main` postdate `v0.4.1`. Test with copied media
> and consult the [release boundaries](ROADMAP.md#what-can-be-tested).

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

The capability review and remaining delivery plan live in [ROADMAP.md](ROADMAP.md).
The movie/TV pipeline, authenticated requests, collections and several opt-in
modules are implemented. Music and import reliability have important changes
after `v0.4.1`; compatibility APIs and migration importers remain partial.

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
