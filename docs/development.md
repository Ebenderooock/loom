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
  ./dist/loom serve  # starts the HTTP server on :1925
```

Open <http://localhost:1925/healthz> — you should see `{"status":"ok"}`.
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

### Go CI shards and coverage

CI runs four race/coverage shards per OS, plus a separate vet and non-race
workflow-engine suite. PRs, pushes to `main` and `v*` release tags use hosted
Ubuntu, macOS and Windows (at most twelve race jobs). Ordinary
`prelease/**`/`prerelease/**` branch pushes use hosted Ubuntu only.
The required `Go (ubuntu-latest)` check, and the corresponding full-matrix OS
checks, aggregate **all** Go jobs, including coverage merging. Any failed,
cancelled or unexpectedly skipped prerequisite fails the gates. Builds still
require these gates and Lint.

`scripts/go-ci.py` discovers packages with `go list -tags disable_libutp`,
subtracts the exact `./internal/workflows/...` discovery, and greedily assigns
each remaining package once. Eight slow-package weights from job
133605996611 seed the balance; new packages get a default weight and are
included automatically. Weights are not elapsed-time predictions: package
times overlap. Inspect the partition or reproduce one shard with:

```bash
CGO_ENABLED=1 python3 -B scripts/go-ci.py plan --shards 4
CGO_ENABLED=1 python3 -B scripts/go-ci.py test --shards 4 --shard 0 --coverage coverage-0.out
CGO_ENABLED=0 go test -tags disable_libutp -count=1 -timeout 120s ./internal/workflows/...
python3 -B -m unittest discover -s scripts -p 'test_*.py'
```

Race runs retain `-race`, `-count=1`, `CGO_ENABLED=1` and `disable_libutp`;
cached test results cannot substitute for execution. Ubuntu uploads uniquely
named `coverage-shard-N` artifacts. CI validates the complete four-profile set,
requires atomic mode, rejects overlapping blocks, and publishes one sorted
`coverage.out` in the existing `coverage` artifact. The independent main-only
coverage job in `integration.yml` is unchanged and does not consume this artifact.

Race shards and the vet/workflows role use separate module/build cache keys,
isolated from setup-go's general-purpose caches used by other jobs. Keys include
OS, architecture, toolchain, role (including race build mode/tags and shard),
`go.mod`/`go.sum` hash and commit SHA. New commits restore the latest compatible
role-specific cache and save updated outputs rather than hitting an immutable
shared primary key forever. Dependency or toolchain changes invalidate the
restore prefix; bump `go-ci-v1` to discard all these caches. Go itself still
validates build inputs, including the two build modes in the vet/workflows role.
There is no claim that the previous shared cache lacked all race build outputs.

Parallelism trades repeated setup, compilation and cache storage/transfer
(including duplicated module caches) and potentially more runner-minutes for
lower wall-clock time. No remote speedup has been measured yet; compare cold
and warm CI runs before changing the four-shard limit or package weights.

Windows fixtures mark large files sparse before extending them, using the
shared `internal/testutil.MarkSparse` helper rather than allocating media-sized
payloads. Windows replacement via Go's `MoveFileEx` can reject an open target
even when its reader permits delete-sharing. The live-path test monitors every
read for missing or incomplete media, requires unchanged identity and bytes on
Windows publication denials, and requires a real replacement after the reader
closes. Separate Windows tests hold ordinary and delete-sharing readers open
and require both files and registration to survive blocked publication.
Replacement never removes the live path first or promises Unix atomic-rename
semantics on Windows; see Microsoft's
[open-file rename restrictions](https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/ntifs/ns-ntifs-_file_rename_information#remarks).

On macOS, replacement publication and registration rollback use `RENAME_SWAP`
instead of ordinary rename to avoid intermittent missing-path errors for
concurrent APFS readers. The live-path test exercises successful replacements
and forced registration failures, requiring complete media throughout and the
original file identity and bytes after rollback. Unsupported swaps fail without
removing the live destination.

## Hot-reload loop

In one terminal, the backend:

```bash
make dev          # uses `air`; rebuilds and restarts on .go changes
```

In another, the frontend:

```bash
cd web
pnpm install
pnpm dev          # Vite on http://localhost:5173, proxies /api to :1925
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
