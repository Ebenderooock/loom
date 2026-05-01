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

## Development setup

Requirements:

- Go 1.23+
- Node 20+ and pnpm (or npm)
- Docker (for compat tests)

```bash
git clone https://github.com/loomctl/loom.git
cd loom
make dev      # runs backend with hot reload + frontend dev server
make test     # full test suite
make lint     # golangci-lint + eslint + prettier
```

## Branching & commits

- Branch off `main`. Name branches `feat/...`, `fix/...`, `docs/...`.
- Use [Conventional Commits](https://www.conventionalcommits.org/):
  `feat(indexers): add cardigann v2 capability negotiation`.
- Squash-merge by default.

## Pull requests

- One concern per PR. Smaller PRs land faster.
- Add or update tests for behavior changes. The CI bar is non-negotiable:
  `go test`, `golangci-lint`, `govulncheck`, `eslint`, frontend type-check,
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

## Reporting security issues

Please do **not** open a public issue. Email security@loomctl.dev (placeholder
— update once domain is provisioned) with details and we will respond within
72 hours.

## Releasing

Maintainers tag `vX.Y.Z` on `main`; GoReleaser produces multi-arch binaries
and signed Docker images on ghcr.io. See `.goreleaser.yaml` and
`.github/workflows/release.yml`.
