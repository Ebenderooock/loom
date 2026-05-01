# Pull request

## Summary

<!-- One or two sentences describing the change and its motivation. -->

Closes #

## Type of change

<!-- Mark the relevant Conventional Commit type for the PR title. -->

- [ ] `feat` — user-visible new behaviour
- [ ] `fix` — bug fix
- [ ] `docs` — documentation only
- [ ] `chore` / `build` / `ci` — tooling
- [ ] `refactor` — internal restructuring, no behaviour change
- [ ] `test` — tests only
- [ ] `perf` — performance
- [ ] `revert` — reverts a previous commit

## Reviewer checklist

- [ ] PR title follows [Conventional Commits](https://www.conventionalcommits.org/).
- [ ] Tests added or updated; `make test` passes locally.
- [ ] `make lint` passes locally.
- [ ] No upstream code copied from Radarr/Sonarr/Prowlarr (clean-room
      policy — see `CONTRIBUTING.md`).
- [ ] No secrets, API keys, or PII added.

## Documentation checklist

> Required for every PR — see
> [`docs/contributing-style.md`](../docs/contributing-style.md#documentation-requirements).

- [ ] New package? Added a `// Package <name>` comment via `doc.go`.
- [ ] New / changed config key? Updated
      [`docs/configuration.md`](../docs/configuration.md).
- [ ] New / changed HTTP route? Updated
      [`api/openapi/loom.yaml`](../api/openapi/loom.yaml) and
      [`docs/api.md`](../docs/api.md).
- [ ] Architectural decision? Added an ADR under
      [`docs/adr/NNNN-*.md`](../docs/adr/) (use
      [`docs/adr/template.md`](../docs/adr/template.md)).
- [ ] Added an entry under `## [Unreleased]` in
      [`CHANGELOG.md`](../CHANGELOG.md).
- [ ] Updated the relevant `docs/<topic>.md` page where user-facing
      behaviour or operational surface changes.

## Notes for reviewers

<!-- Anything reviewers should look at first, screenshots for UI work,
     migration impact, deploy notes, etc. -->
