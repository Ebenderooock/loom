# Loom Documentation

Welcome. Loom is a Go-based, modular-monolith replacement for
Radarr + Sonarr + Prowlarr. These pages cover the project as it stands today
(early Phase 1) and lay down the structure new docs will grow into.

## For users

| Page | What it covers |
|---|---|
| [Quickstart (README)](../README.md#quickstart) | Run Loom in 60 seconds with Docker. |
| [Configuration](configuration.md) | Every config key, env var, default, and whether it hot-reloads. |
| [Deployment](deployment.md) | Docker / Compose / Kubernetes / bare-metal / reverse proxy. |
| [Observability](observability.md) | Logs, `/metrics`, OpenTelemetry, pprof. |
| [Storage](storage.md) | SQLite vs Postgres, migrations, backups. |
| [Security](security.md) | Threat model, secrets, vuln reporting. |
| [Migration from Radarr/Sonarr/Prowlarr](migration.md) | _Stub — lands in Phase 8._ |

## For contributors

| Page | What it covers |
|---|---|
| [Development](development.md) | Clone → build → test → hot-reload loop. |
| [Architecture](architecture.md) | Modular monolith, module boundaries, event bus, split-mode. |
| [API](api.md) | Native `/api/v1/*` and (Phase 7) wire-compat surfaces. |
| [Authentication](auth.md) | _Stub — lands in Phase 1c._ |
| [Contributing & code style](contributing-style.md) | Commit format, PR review checklist, doc-update policy. |
| [Architecture decision records](adr/) | Numbered ADRs (ADR-0001 … ADR-0010). |
| [Web UI — Indexers & Proxies](web/indexers-ui.md) | Walkthrough of the frontend management pages. |

## For maintainers

- [Roadmap](../ROADMAP.md) — public mirror of the 11-phase delivery plan.
- [Changelog](../CHANGELOG.md) — Keep a Changelog format.
- [Security policy](../SECURITY.md) — coordinated disclosure process.
- [Code of conduct](../CODE_OF_CONDUCT.md).

## Documentation site

The site this directory powers can be served locally; see
[`website/`](website/README.md) for the bootstrap procedure.
