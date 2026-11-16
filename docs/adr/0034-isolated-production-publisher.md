# ADR-0034: Isolate production publishing authority

- Status: Accepted
- Date: 2026-11-17
- Deciders: @Ebenderooock

## Context

A workflow on a writable Loom branch can request `packages: write`, even when
the default workflow token is read-only. Repository-wide GHCR write access
therefore lets an unapproved workflow publish production images. Publishing
must remain available for `main`, both prerelease branch spellings, and release
tags, using GitHub Free.

## Decision

Move production publishing into the public `Ebenderooock/loom-publisher`
repository. Protect publisher `main` with reviewed pull requests and required
checks. Keep the GHCR publishing PAT and source GitHub App key only in its
`production-publish` environment, restricted to branch `main` without
administrator bypass. Remove automatic GHCR Write/Admin access for both
repositories, including inherited access.

The publisher independently verifies the current allowed source ref and its
successful push CI run. It builds source code without publishing credentials,
then validates and publishes outputs in separate trusted jobs. Loom workflows
only observe App-authenticated publication statuses; releases, downloads, and
badges remain on Loom. See [deployment](../deployment.md#trusted-publishing) for
the streams, credential permissions, and coordinated cutover procedure.

## Consequences

### Positive

- Unapproved branch workflows cannot obtain production publishing authority
  merely by requesting a more privileged `GITHUB_TOKEN`.
- Approved publishing code, source CI, publication runs, and commit statuses
  provide a traceable release path without a paid GitHub plan.

### Negative / trade-offs

- A second repository, GitHub App, and expiring PAT require maintenance.
- Scheduled discovery introduces a delay; public scheduled workflows require
  monitoring for inactivity-based disabling.
- Optional build credentials such as `TVDB_APIKEY` remain accessible to source
  build code and must never carry publishing authority.
- These controls support audit evidence but do not establish SOC 2 compliance.

### Neutral

- Publishing remains automatic after approved source changes.
- Package access changes, source migration, and publisher activation must be
  coordinated; enabling the environment alone does not remove legacy writers.

## Alternatives considered

- Read-only workflow defaults: modified workflows can request package-write
  permissions, so defaults do not enforce the required boundary.
- A protected credential inside Loom: viable with correct package access
  isolation, but keeps publishing infrastructure within the source repository.
- A publisher with automatic GHCR write access: recreates the unapproved-branch
  weakness in the publisher repository.
