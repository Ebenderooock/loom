<!--
Sync Impact Report
  Version change: 0.0.0 → 1.0.0
  Modified principles: none (initial adoption)
  Added sections:
    - I. Code Quality
    - II. Testing Standards
    - III. User Experience Consistency
    - IV. Performance Requirements
    - Quality Gates (Section 2)
    - Development Workflow (Section 3)
    - Governance
  Removed sections: none
  Templates requiring updates:
    - .specify/templates/plan-template.md ✅ (Constitution Check aligns)
    - .specify/templates/spec-template.md ✅ (no updates needed)
    - .specify/templates/tasks-template.md ✅ (no updates needed)
  Follow-up TODOs: none
-->
# Loom Constitution

## Core Principles

### I. Code Quality

- All production code MUST pass `golangci-lint` with the project's
  `.golangci.yml` configuration and zero warnings before merge.
- Every exported function, type, and method MUST have a Go doc comment
  that states its purpose and any non-obvious behaviour.
- Functions MUST NOT exceed 60 statements. Packages MUST have a single,
  clear responsibility. When a package grows beyond its scope, it MUST
  be split.
- All SQL queries MUST be generated via `sqlc`. Hand-written SQL in
  application code is prohibited.
- Error handling MUST be explicit: never discard errors silently. Wrap
  errors with `fmt.Errorf("context: %w", err)` to preserve the chain.
- Structured logging via `slog` is mandatory. Log messages MUST include
  relevant context fields and MUST NOT contain PII.

### II. Testing Standards

- Every new feature or bug fix MUST include tests that exercise the
  changed behaviour. Pull requests that reduce overall coverage below
  the current baseline MUST NOT be merged.
- Unit tests MUST be deterministic, isolated, and fast (<1 s each).
  Tests MUST NOT depend on network, filesystem state, or wall-clock
  time unless explicitly marked as integration tests.
- Integration tests MUST cover: database migrations, HTTP handler
  contracts, inter-module communication, and external service
  boundaries (using test doubles or containers).
- Table-driven tests are the default style for Go tests. Each test case
  MUST have a descriptive name that documents the scenario under test.
- Test helpers MUST call `t.Helper()`. Assertions MUST produce clear
  failure messages that identify expected vs. actual values.

### III. User Experience Consistency

- The frontend MUST present a single, unified interface across movies,
  series, indexers, and all future media types. Navigation patterns,
  terminology, and visual hierarchy MUST remain consistent.
- All interactive elements MUST meet WCAG 2.1 AA accessibility
  standards: proper ARIA labels, keyboard navigation, sufficient
  colour contrast (≥4.5:1 for text), and focus indicators.
- UI components MUST use the project's shadcn/ui design system and
  Tailwind theme tokens. Custom one-off styles are prohibited unless
  no existing component or token covers the use case.
- Every user-facing action MUST provide visual feedback within 100 ms
  (loading indicator, optimistic update, or skeleton). Error states
  MUST display actionable messages — never raw error codes or stack
  traces.
- API responses consumed by the frontend MUST follow a consistent
  envelope structure. Breaking changes to public API shapes MUST go
  through a deprecation cycle with at least one minor release of
  overlap.

### IV. Performance Requirements

- API endpoints MUST respond within 200 ms at p95 under normal load
  (≤100 concurrent users). Endpoints exceeding this budget MUST be
  profiled and optimised before merge.
- The Go binary's baseline memory footprint MUST remain below 128 MB
  RSS at idle with an empty library. Memory-intensive operations MUST
  use bounded buffers or streaming patterns.
- Database queries MUST NOT perform full table scans on tables expected
  to exceed 10 000 rows. All queries MUST have appropriate indexes
  verified via `EXPLAIN` output.
- Frontend bundles MUST maintain a Lighthouse Performance score ≥90 on
  simulated mobile (Moto G Power, slow 4G). Lazy loading MUST be used
  for routes and heavy components.
- Container images MUST NOT exceed 50 MB compressed. Build times (CI)
  MUST remain under 5 minutes for the full pipeline. Regressions in
  either metric MUST be justified and approved.

## Quality Gates

All pull requests MUST satisfy every gate before merge:

1. **Lint gate**: `golangci-lint run` and `eslint` (frontend) pass
   with zero errors and zero warnings.
2. **Test gate**: `go test ./...` and `npm test` (frontend) pass with
   zero failures. Flaky tests MUST be quarantined, not skipped.
3. **Build gate**: `go build ./cmd/...` and `npm run build` succeed.
   Docker image builds MUST also succeed.
4. **Review gate**: At least one approving review from a maintainer.
   Reviews MUST verify constitutional compliance.
5. **Performance gate**: For changes touching hot paths or database
   queries, benchmarks MUST be included and MUST NOT show >10%
   regression vs. the main branch baseline.

## Development Workflow

- Feature work MUST happen on dedicated branches named
  `<issue-number>-<short-description>` (e.g., `42-add-series-search`).
- Commits MUST follow Conventional Commits format:
  `<type>(<scope>): <description>` (e.g., `feat(indexer): add Torznab
  support`).
- The `main` branch MUST always be in a deployable state. Direct pushes
  to `main` are prohibited.
- Migrations MUST be additive and backward-compatible. Destructive
  schema changes require a two-phase migration strategy.
- Observability instrumentation (traces, metrics, structured logs) MUST
  be added alongside new features, not deferred as follow-up work.

## Governance

This constitution is the authoritative source for Loom's engineering
standards. It supersedes ad-hoc practices, tribal knowledge, and
conflicting documentation.

- **Amendments**: Any change to this constitution MUST be proposed via
  pull request, reviewed by at least two maintainers, and include a
  migration plan for existing code that would be affected.
- **Versioning**: The constitution follows semantic versioning. MAJOR
  bumps for principle removals or incompatible redefinitions, MINOR for
  new principles or material expansions, PATCH for clarifications.
- **Compliance reviews**: All code reviews MUST include a constitution
  compliance check. Reviewers MUST flag violations explicitly.
- **Exceptions**: Temporary deviations MUST be documented in the PR
  description with a linked follow-up issue for remediation.

**Version**: 1.0.0 | **Ratified**: 2026-05-18 | **Last Amended**: 2026-05-18
