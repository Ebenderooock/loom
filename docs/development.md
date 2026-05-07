# Development

## Prerequisites

- **Go 1.23 or later** (matching `go.mod`).
- **Node 20 LTS** and **pnpm** (for the React frontend in `web/`).
- **Docker** (for compat tests and local container builds).
- **sqlc** (optional — only needed when regenerating typed queries):
  `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`.
- **air** (optional — auto-reload backend during dev):
  `go install github.com/air-verse/air@latest`.
- **golangci-lint** (auto-installed by `make lint` if missing).

## First clone

```bash
git clone https://github.com/ebenderooock/loom.git
cd loom
make build           # produces dist/loom
./dist/loom version

mkdir -p ./run
LOOM_CONFIG_DIR=./run \
LOOM_DATA_DIR=./run \
LOOM_STORAGE_SQLITE_PATH=./run/loom.db \
  ./dist/loom serve  # starts the HTTP server on :8989
```

Open <http://localhost:8989/healthz> — you should see `{"status":"ok"}`.
The default `storage.sqlite.path` is `/data/loom.db` (designed for the
container layout); the env-var overrides above make local runs work
without root.

## Common tasks

| Task | Command |
|---|---|
| Build the binary | `make build` |
| Run tests with race detector | `make test` |
| Quick tests (no race) | `make test-short` |
| Lint Go | `make lint` |
| Format Go | `make fmt` |
| Build the local Docker image | `make docker` |
| Run the binary | `make run` |

The full `make help` lists every target with descriptions.

## Hot-reload loop

In one terminal, the backend:

```bash
make dev          # uses `air`; rebuilds and restarts on .go changes
```

In another, the frontend:

```bash
cd web
pnpm install
pnpm dev          # Vite on http://localhost:5173, proxies /api to :8989
```

When the React build is consumed by the backend (production image), the
backend embeds it via a build tag; in dev you just point the browser at
the Vite server.

## Adding a migration

1. `internal/storage/migrations/sqlite/NNN_<summary>.sql` with `-- +goose Up`
   and `-- +goose Down` blocks.
2. Mirror in `internal/storage/migrations/postgres/NNN_<summary>.sql`.
3. Add the queries under `internal/storage/queries/<engine>/<table>.sql`.
4. `sqlc generate` to refresh `internal/storage/db/<engine>/`.
5. `make test` — the suite runs against both engines.

See [storage.md](storage.md#adding-a-migration) for full detail.

## Adding a route

1. Implement the handler in `internal/server/` (or a module package).
2. Mount it on the chi router in `internal/server/server.go`.
3. Add it to [`api/openapi/loom.yaml`](../api/openapi/loom.yaml).
4. Update [api.md](api.md) and write a test.

The PR template enforces every step.

## Adding a config key

1. Add the field to the relevant struct in
   `internal/kernel/config/config.go`.
2. Set its default in `applyDefaults`.
3. Validate it in `Config.Validate` if it can take an invalid value.
4. Document it in [docs/configuration.md](configuration.md) with the
   YAML path, env var, default, type, and hot-reload status.
5. Add a CHANGELOG entry.

## Adding documentation

- New ADR? Copy `docs/adr/template.md` to `docs/adr/NNNN-<slug>.md`.
- New page? Drop a Markdown file under `docs/` and link to it from
  [`docs/README.md`](README.md).
- New package? Add a `doc.go` with a `// Package <name>` comment.

## Running the lint pass

```bash
make lint                              # golangci-lint
npx --yes markdownlint-cli2 "**/*.md" "!node_modules/**" "!web/node_modules/**"
npx --yes @redocly/cli lint api/openapi/loom.yaml
```

The CI workflow (`.github/workflows/ci.yml`) runs the same set.
