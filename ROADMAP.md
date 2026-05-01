# Roadmap

This is the public mirror of Loom's 11-phase delivery plan. Phases ship
releasable artifacts (alpha → beta → rc → 1.0) and overlap where they
are independent.

Legend: ✅ done · 🚧 in progress · ⏳ planned

---

## ✅ Phase 0 — Repo & engineering foundations

- ✅ Initialise Go module, repo skeleton, AGPL-3.0 license.
- ✅ CI: GitHub Actions for `go test`, `golangci-lint`, `govulncheck`,
  frontend lint/build.
- ✅ Release pipeline: GoReleaser → multi-arch binaries + Docker images
  on `ghcr.io`.
- ✅ ADR directory with the first five ADRs (language, storage, API,
  auth, observability).
- ✅ Contributor docs: `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`,
  `SECURITY.md`, issue templates.

## 🚧 Phase 1 — Platform kernel

- ✅ Config: layered (defaults → file → env → flags) via Viper;
  hot-reload for safe keys.
- ✅ Logging: `slog` JSON; PII redaction.
- ✅ Telemetry: OpenTelemetry SDK; OTLP exporter; Prometheus `/metrics`;
  pprof gated by config.
- ✅ Storage: SQLite + Postgres, goose migrations, sqlc query generation.
- ✅ Scheduler: in-memory cron + jittered intervals + one-shot tasks
  (persistence to follow).
- ✅ Event bus: in-process channels (NATS in split-mode lands in Phase 11).
- ✅ HTTP server: chi router, request-ID, structured access logs, panic
  recovery, gzip, ETag.
- ✅ Health endpoints: `/healthz`, `/readyz`, `/livez`.
- ✅ React app skeleton: Vite + TS + TanStack Router/Query + shadcn/ui +
  Tailwind + Storybook + Playwright.
- 🚧 Auth: forms login (argon2id), API keys, OIDC (Authelia / Authentik /
  Keycloak / Entra), reverse-proxy header trust.

## ⏳ Phase 2 — Indexer subsystem (Prowlarr-equivalent)

Cardigann YAML loader, Newznab/Torznab in/out, search aggregation,
indexer health, proxies + FlareSolverr, manual search UI.

## ⏳ Phase 3 — Download client adapters

qBittorrent, Transmission, Deluge, rTorrent, SABnzbd, NZBGet. Queue,
remote-path mappings, blocklist, redownload-on-failure.

## ⏳ Phase 4 — Metadata providers

TMDB, TheTVDB v4, IMDB, Trakt, AniDB + AniList, image proxy/cache.

## ⏳ Phase 5 — Movies module (Radarr-equivalent)

Library, root folders, quality profiles, Custom Formats, parser,
RSS sync, import pipeline (hardlink/EXDEV-aware), lists, collections,
calendar.

## ⏳ Phase 6 — Series module (Sonarr-equivalent)

Series → seasons → episodes, season packs, anime mapping, calendar,
Sonarr v3 + v4 API shape parity.

## ⏳ Phase 7 — Wire-compatibility surfaces

`/api/v3/*` Radarr + Sonarr, `/api/v1/*` Prowlarr, Bazarr endpoints,
compat acceptance test suite running real downstream apps in CI.

## ⏳ Phase 8 — Migration tooling

`loom migrate import --from {radarr,sonarr,prowlarr}`, dry-run + diff,
idempotent re-run, frontend wizard, side-by-side mode.

## ⏳ Phase 9 — Frontend feature parity & UX modernisation

Unified library, discover, activity centre, calendar, settings deep-link
search, themes, mobile-first, ⌘K palette, WCAG 2.2 AA, i18n.

## ⏳ Phase 10 — Notifications, automation & extensibility

Discord / Slack / Telegram / Pushover / Pushbullet / Gotify / ntfy /
Apprise / Email / Webhook / Notifiarr / MQTT, custom scripts (sandboxed),
plugin SDK (out-of-process gRPC).

## ⏳ Phase 11 — Deployment, hardening & 1.0

Distroless multi-arch images (non-root, RO rootfs), Helm chart,
Kustomize overlays, Grafana dashboards, backup/restore CLI, SBOM
(syft), image signing (cosign), SLSA provenance, performance
benchmarks (Pi 5 + small VPS), public Docusaurus site, **1.0 release**.
