# Contributing to Loom

Thanks for considering a contribution! Loom is a community-driven replacement
of the *arr stack, and every fix, doc improvement, and feature counts.

## Ground rules

1. **No upstream code copied from Radarr/Sonarr/Prowlarr.** Loom is a
   clean-room reimplementation. We reference public API specs, docs, and
   user-facing config files only. If you've worked on those projects and
   want to contribute, that's great — but please write fresh code here.
2. Be kind. See `CODE_OF_CONDUCT.md`.
3. Open an issue (or comment on an existing one) before large changes so
   we can align on direction.

---

## Prerequisites

### Go (backend)

Loom requires **Go 1.25+** (the `go.mod` specifies `go 1.25.7`).

| Platform | Install |
|---|---|
| **macOS** | `brew install go` |
| **Ubuntu / Debian** | See [go.dev/doc/install](https://go.dev/doc/install) — the distro package is often outdated. Download the tarball instead. |
| **Fedora / RHEL** | `sudo dnf install golang` (check version ≥ 1.25) |
| **Windows** | Download the MSI from [go.dev/dl](https://go.dev/dl/) or `winget install GoLang.Go` |
| **Raspberry Pi (ARM)** | Download the `linux-arm64` or `linux-armv6l` tarball from [go.dev/dl](https://go.dev/dl/) |

Verify: `go version` should print `go1.25` or newer.

### Node.js (frontend)

The React UI requires **Node 20+** and npm.

| Platform | Install |
|---|---|
| **macOS** | `brew install node` |
| **Ubuntu / Debian** | [NodeSource](https://github.com/nodesource/distributions): `curl -fsSL https://deb.nodesource.com/setup_20.x \| sudo bash - && sudo apt install -y nodejs` |
| **Windows** | Download from [nodejs.org](https://nodejs.org/) or `winget install OpenJS.NodeJS.LTS` |
| **Any (via nvm)** | `nvm install 20 && nvm use 20` — recommended if you work on multiple Node projects |

Verify: `node --version` (≥ 20) and `npm --version` (≥ 9).

### Optional tools

| Tool | Purpose | Install |
|---|---|---|
| **Docker** | Container builds, compat tests | [docs.docker.com/get-docker](https://docs.docker.com/get-docker/) |
| **golangci-lint** | Go linting (auto-installed by `make lint`) | `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` |
| **air** | Backend hot-reload (auto-installed by `make dev`) | `go install github.com/air-verse/air@latest` |
| **sqlc** | Regenerate typed DB queries (only if changing SQL) | [sqlc.dev](https://sqlc.dev/) |

---

## Getting started

```bash
# 1. Clone the repo
git clone https://github.com/ebenderooock/loom.git
cd loom

# 2. Install frontend dependencies
cd web && npm install && cd ..

# 3. Run in development mode (two terminals)
# Terminal 1 — backend with hot-reload:
make dev
# Terminal 2 — frontend dev server (Vite, port 5173):
cd web && npm run dev

# 4. Open http://localhost:5173 in your browser
#    Default credentials: admin / admin
```

### Building a production binary

```bash
# Single binary with embedded React UI (serves everything on :1925)
make build-all

# Run it
./dist/loom serve

# Cross-compile for other platforms (RPi, Windows, Linux)
make cross
# Produces: dist/loom-linux-amd64, loom-linux-arm64, loom-linux-armv7,
#           loom-darwin-arm64, loom-darwin-amd64, loom-windows-amd64.exe
```

### Docker

```bash
make docker                     # builds loom:dev image
docker compose up -d            # runs loom + qbittorrent + prometheus + grafana
```

### Running tests

```bash
make test          # full test suite with race detector
make test-short    # fast subset
make lint          # golangci-lint
cd web && npx tsc --noEmit   # frontend type-check
```

---

## Project structure

```
cmd/loom/           # CLI entry point (serve, healthcheck)
internal/           # All backend packages (server, storage, indexers, etc.)
web/                # React frontend (Vite + TanStack Router + shadcn)
config/             # Runtime config directory (created on first run)
deploy/             # Dockerfile, Helm chart, Kustomize, Prometheus config
scripts/            # Helper scripts (sync Prowlarr definitions, etc.)
docs/               # Architecture decision records, design docs
```

---

## Branching & commits

- Branch off `main`. Name branches `feat/...`, `fix/...`, `docs/...`.
- Use [Conventional Commits](https://www.conventionalcommits.org/):
  `feat(indexers): add cardigann v2 capability negotiation`.
- Squash-merge by default.

## Pull requests

- One concern per PR. Smaller PRs land faster.
- Add or update tests for behavior changes. The CI bar is non-negotiable:
  `go test`, `golangci-lint`, `govulncheck`, frontend type-check,
  and parser-fixture tests must all pass.
- Update relevant ADRs (`docs/adr/`) if your change crosses architectural
  boundaries.

## Architecture decision records (ADRs)

We record significant decisions as numbered ADRs under `docs/adr/`. Use
`docs/adr/template.md` to draft new ones and reference them from PRs.

## Testing the wire-compatibility surface

We run real downstream apps (Overseerr, Bazarr, Notifiarr) against Loom in
docker-compose during CI. If your PR touches a `/api/v3` or `/api/v1`
handler, run the compat suite locally:

```bash
make test-compat
```

## Configuration

Loom auto-creates a config directory on first run (`./config/` by default).

| Setting | Env var | Default |
|---|---|---|
| HTTP listen address | `LOOM_HTTP_ADDR` | `:1925` |
| Config directory | `LOOM_CONFIG_DIR` | `./config` |
| Data directory | `LOOM_DATA_DIR` | `./config` |
| Log level | `LOOM_LOG_LEVEL` | `info` |
| OTel endpoint | `LOOM_OTEL_ENDPOINT` | (disabled) |
| Prometheus metrics | `LOOM_PROMETHEUS` | `true` |

See `config/loom.yaml` for the full configuration reference.

## Reporting security issues

Please do **not** open a public issue. Email security@ebenderooock.dev (placeholder
— update once domain is provisioned) with details and we will respond within
72 hours.

## Releasing

Maintainers tag `vX.Y.Z` on `main`; GoReleaser produces multi-arch binaries
and signed Docker images on ghcr.io. See `.goreleaser.yaml` and
`.github/workflows/release.yml`.
