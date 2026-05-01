# Changelog

All notable changes to Loom are documented here.

The format is based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- **Phase 0 — bootstrap.** Repository skeleton, AGPL-3.0 license,
  `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, GitHub issue
  templates, GoReleaser config, multi-arch Dockerfile, and the first
  five ADRs (language, storage, API, auth, observability). (`2d291e6`)
- **Phase 1a — platform kernel.** Layered Viper config with hot-reload,
  `slog` JSON logging with PII redaction, OpenTelemetry SDK + OTLP
  exporter, Prometheus `/metrics`, pprof gating, in-process event bus,
  cron-style scheduler, chi-based HTTP server with health probes
  (`/healthz`, `/livez`, `/readyz`) and `/api/v1/system/status`. (`f0ea941`)
- **Phase 1b — storage layer.** Two-engine database abstraction
  (SQLite via `modernc.org/sqlite`, Postgres via `jackc/pgx`), embedded
  goose migrations per engine, sqlc-generated query packages, schema
  meta table, and a unified `Querier` interface. (`10bdae3`)
- **Phase 1e — frontend scaffold.** React 18 + TypeScript + Vite +
  TanStack Router/Query + shadcn/ui + Tailwind app under `web/`,
  including ESLint flat config, Prettier, Storybook, Playwright. (`1b2fb09`)
- **Phase 1d — persistent scheduler.** Cron-driven scheduler in
  `internal/kernel/scheduler` backed by `robfig/cron/v3` and the
  `scheduled_jobs` table. Run history (`last_run_at`, `next_run_at`,
  `last_status`, `last_error`) survives restarts. Idempotent
  `Register`, per-job `TryLock` to prevent overlapping runs,
  bounded-grace shutdown, configurable timezone, and a built-in
  `system.housekeeping` job (`PRAGMA optimize` / `VACUUM ANALYZE`
  every 6 hours). New config keys: `scheduler.enabled`,
  `scheduler.timezone`, `scheduler.shutdown_grace`. Migration
  `0005_scheduled_jobs_status.sql` adds `enabled`, `last_status`,
  `last_error` columns on both engines. ADR-0006.
- **Documentation baseline.** `docs/` developer documentation
  (architecture, configuration, observability, storage, API,
  development, deployment, security), per-package `doc.go` comments,
  OpenAPI 3.1 spec stub at `api/openapi/loom.yaml`, public roadmap,
  PR template, code-owners placeholder.

### Changed

- **Scheduler API replaced.** The Phase-1a in-memory ticker scheduler
  has been removed. `scheduler.Register` now takes a unique job name,
  a cron expression, a `HandlerFunc(ctx) error`, and an optional
  payload, and persists through a `Store`. Callers must construct the
  scheduler via `scheduler.New(cfg, store, logger, clock)`; the
  binary wires this in `cmd/loom/scheduler.go`.

### Fixed

- *(none yet)*

### Security

- *(none yet)*

[Unreleased]: https://github.com/loomctl/loom/compare/HEAD
