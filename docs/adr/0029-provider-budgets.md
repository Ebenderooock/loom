# ADR-0029: Persistent explicit provider/account budgets

- Status: Accepted
- Date: 2026-10-24
- Deciders: @loom-maintainers

## Context

Rolling-search counters are per-definition, in memory, and intentionally scoped
to scheduler policy. Multiple definitions can consume one real provider account
allowance. Transport throttling and user request quotas model different limits.

## Decision

Use explicit operator-assigned account groups with persistent SQL-atomic
admission. Reserve at the existing registry query boundary and reuse that
reservation for the first outbound HTTP attempt. Wrap the transport inside the
existing throttle to account for retries and multi-path searches. Persist
supported remaining/reset signals conservatively; manual and streaming searches
follow the same policy. Neither credentials nor hostname-derived identities
form budget keys. Administrator configuration retains all accounting state;
there is no search-time provider-limit bypass.

## Consequences

### Positive

- Concurrent callers and restarts cannot reset or multiply shared allowances.
- Existing scheduler quotas, circuit breakers and transport throttles remain
  independent.
- Diagnostics distinguish budget skips from failures and expose resume times.

### Negative / trade-offs

- Failed and ambiguous attempts remain charged.
- Unknown provider reset times fail closed and need operational intervention.
- Custom indexer transports need the shared transport chain for per-HTTP-attempt
  accounting; query admission still applies.
- Capability probes outside searches and downloads are not covered.

### Neutral

- Unconfigured definitions retain their existing behavior.
- Database migrations add budget and membership tables to both engines.

## Alternatives considered

- Hostname grouping merges unrelated accounts and fails with aliases.
- Credential-derived identities couple accounting to secrets and rotation.
- In-memory counters lose state on restart and cannot coordinate processes.
- Raising reported allowances from each response can grant calls when responses
  arrive out of order; signals therefore only tighten a live allowance.
