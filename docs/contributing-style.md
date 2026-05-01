# Contributing & code style

`CONTRIBUTING.md` covers the basics — fork, branch, PR. This page goes
deeper on style, commit format, review expectations, and the
documentation-update policy.

## Code style

### Go

- **Formatter:** `gofmt` + `goimports` (with `local-prefixes:
  github.com/loomctl/loom`). Configured in `.golangci.yml`'s `formatters`
  section.
- **Lint set:** `errcheck`, `govet`, `ineffassign`, `staticcheck`,
  `unused`, `gocritic`, `gosec`, `misspell`, `unconvert`, `unparam`,
  `revive`, `bodyclose`, `contextcheck`, `errorlint`, `nilerr`,
  `prealloc`. The full list lives in
  [`.golangci.yml`](../.golangci.yml).
- Exported symbols must have doc comments (`revive`'s `exported` rule).
- Errors are wrapped with `fmt.Errorf("...: %w", err)` and inspected
  with `errors.Is`/`errors.As`.

### TypeScript / React

- ESLint flat config and Prettier; both run in CI. See
  [`web/`](../web) for the configuration files.
- Components live under `web/src/components/`; routes under
  `web/src/routes/`; data hooks under `web/src/hooks/`.

## Commit messages

[Conventional Commits](https://www.conventionalcommits.org/). Allowed
type prefixes:

| Type | When to use it |
|---|---|
| `feat` | A new feature or user-visible behaviour. |
| `fix` | A bug fix. |
| `docs` | Documentation-only changes. |
| `chore` | Maintenance — dep bumps, tooling, repo hygiene. |
| `refactor` | Code change that is neither a feature nor a bug fix. |
| `test` | Adding or fixing tests. |
| `ci` | CI configuration changes. |
| `build` | Build system / Dockerfile / GoReleaser changes. |
| `perf` | Performance improvement. |
| `revert` | Reverts a previous commit. |

Example: `feat(indexers): add cardigann v2 capability negotiation`.

Use a `BREAKING CHANGE:` footer to call out wire-API breaks.

## Pull-request review checklist

A reviewer should be able to answer **yes** to all of these before
merging:

- [ ] CI is green.
- [ ] Tests cover the change (happy + at least one failure mode).
- [ ] No upstream code copied from Radarr/Sonarr/Prowlarr (clean-room
      policy — see `CONTRIBUTING.md`).
- [ ] Public Go symbols are documented.
- [ ] Naming is consistent with surrounding code.
- [ ] Errors are wrapped with context and inspected with `errors.Is`.
- [ ] No secrets, API keys, or PII added (logged or stored in repo).
- [ ] Doc-update policy followed (see below).
- [ ] CHANGELOG entry added under `[Unreleased]`.
- [ ] PR title follows Conventional Commits.

## Readability principles

We treat readable code and readable docs as a feature, not a polish step.
A reviewer should be able to skim a file once and understand what it
does. Apply this filter on every PR — both yours and your reviewees.

### Code readability

- **Names tell what they do.** `findMovieByTMDBID` over `lookup`,
  `pendingDownloads` over `pd`. Single-letter names are reserved for
  loop indices and standard receivers.
- **One concept per package.** A package whose name needs an "and" is
  two packages. Keep imports flowing in one direction.
- **Functions stay short.** Aim for ~30 lines; split aggressively.
  `gocyclo` and `funlen` (when we enable them) will police this.
- **Errors read as sentences and carry context.** `fmt.Errorf("open
  config %q: %w", path, err)`, never `errors.New("error")`. Inspect
  with `errors.Is` / `errors.As`, not string matching.
- **Comments explain why, not what.** The code shows the what. Save
  comments for non-obvious decisions, invariants, or links to the
  upstream issue.
- **Public symbols are documented.** Every exported Go identifier has
  a `// Name does X` comment (`revive`'s `exported` rule). Same for
  exported TS types.
- **No clever one-liners.** A clear five-line block beats a
  six-function pipeline that needs a debugger to read.
- **TypeScript is strict.** `strict: true` in `tsconfig.json`; no
  `any` without a justifying comment; prefer `unknown` + a narrowing
  type guard.

### Documentation readability

- **Plain English first.** Write for a competent friend who has not
  read this codebase. Define jargon the first time it appears.
- **Show, don't tell.** Every config key has an example value. Every
  endpoint has a `curl` line. Every command is pasteable as-is.
- **Active voice, present tense.** "Loom reads the config" beats
  "the config is read by Loom".
- **Front-load the answer.** Lead each section with the one-line
  takeaway; details follow.
- **One topic per page, kept short.** If a doc page passes ~600
  lines, split it. Cross-link instead of duplicating.
- **Examples are tested.** If a snippet doesn't run today (e.g.
  unreleased Docker image, future phase), label it as such next to
  the snippet.
- **Status is honest.** Use 🚧 / ⏳ / ✅ tags or "Phase N" labels for
  not-yet-built surfaces. Don't pretend a placeholder is real.

## Documentation requirements

Every PR must:

- Add a `// Package <name>` comment via `doc.go` to any new package.
- Update [`docs/configuration.md`](configuration.md) for any new or
  changed config key.
- Update [`api/openapi/loom.yaml`](../api/openapi/loom.yaml) for any new
  or changed HTTP route.
- Add an ADR under [`docs/adr/NNNN-<slug>.md`](adr/) for non-trivial
  design decisions (use [`docs/adr/template.md`](adr/template.md)).
- Add an entry to [`CHANGELOG.md`](../CHANGELOG.md) under
  `## [Unreleased]`.
- Update the relevant `docs/<topic>.md` page where user-facing
  behaviour, deployment shape, or operational surface changes.

These items are mirrored as a checklist in
[`.github/PULL_REQUEST_TEMPLATE.md`](../.github/PULL_REQUEST_TEMPLATE.md)
and are enforced at review time, not by CI bots.

## Releasing

Maintainers tag `vX.Y.Z` on `main`. GoReleaser produces the multi-arch
binaries and signed Docker images via
[`.github/workflows/release.yml`](../.github/workflows/release.yml). Tagging
moves the `[Unreleased]` block in `CHANGELOG.md` under the new version.
