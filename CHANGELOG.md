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
- **Documentation baseline.** `docs/` developer documentation
  (architecture, configuration, observability, storage, API,
  development, deployment, security), per-package `doc.go` comments,
  OpenAPI 3.1 spec stub at `api/openapi/loom.yaml`, public roadmap,
  PR template, code-owners placeholder.

### Changed

- *(none yet)*

### Fixed

- *(none yet)*

### Security

- *(none yet)*

[Unreleased]: https://github.com/loomctl/loom/compare/HEAD
