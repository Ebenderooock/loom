# Provider API budgets

Provider budgets persist a shared provider/account allowance across restarts.
They are separate from transport requests-per-minute, circuit-breaker cooldowns,
rolling-search quotas, and user request quotas.
The design rationale is in [ADR-0029](adr/0029-provider-budgets.md).

## Configure an account group

Create indexer definitions first, then assign those sharing the **same provider
account** to one group. Use an opaque operator label such as `account-a`, never
an API key, username, URL, or other credential. Loom does not infer account
identity from hostname or hash credentials. Different accounts on the same host
remain independent. A definition can belong to only one group.

```sh
curl -X PUT http://localhost:1925/api/v1/indexers/provider-budgets/account-a \
  -H "X-Api-Key: $LOOM_API_KEY" -H 'Content-Type: application/json' \
  -d '{"allowance":100,"window_seconds":86400,"indexer_ids":["indexer-one","indexer-two"]}'

curl http://localhost:1925/api/v1/indexers/provider-budgets \
  -H "X-Api-Key: $LOOM_API_KEY"
```

Both endpoints require an administrator when authentication is enabled.
`PUT` replaces the group's complete membership and configured limits, atomically.
It returns 204. IDs allow 1-64 ASCII letters, digits, underscores and hyphens
(first character must be alphanumeric). `allowance` is 1-1,000,000,000 and
`window_seconds` is 1-31,622,400. Duplicate, nonexistent, or already-assigned
members reject the whole update. To move a member, first remove it from its old
group. An empty member list detaches all members but retains the group's state.

The first window starts when the group is created. At or after its boundary,
the next admitted attempt starts a fresh window. Changing limits or membership
does **not** reset consumed calls or provider signals. A changed window length
applies at the next configured reset. List responses show persisted state;
an expired window is rolled forward on admission, not by reading the list.
Unconfigured definitions retain existing behavior, including the separate
in-memory rolling-search quota.

## What consumes allowance

Automatic, manual, compatibility API and streaming searches use the same
provider budget. Every admitted indexer query consumes a call, including ID,
title and alternate-title queries, provider failures and empty results.
Cancelled queries and queries rejected by registry eligibility checks do not
consume calls. Once reserved, a call is not refunded: a timeout or cancellation
cannot prove the provider did not receive it.

For built-in HTTP clients, transport retries, extra search paths and lazy
requests inside a search consume additional calls. The existing throttle wraps
budget accounting, so retries cannot bypass an exhausted allowance. The first
HTTP attempt uses the query's reservation rather than charging twice.
Custom indexers still consume one reservation per query; custom transports must
use `TransportForDefinition` to account for additional outbound attempts.
Connectivity tests, downloads, and capability probes outside a search are not
search attempts and are outside this budget.

Admission uses a conditional SQL update, not a read-then-increment counter.
Definitions in one group and concurrent processes sharing the database cannot
multiply that group's allowance. SQLite and PostgreSQL have matching schemas.
Accounting failures block searches and report an error rather than silently
allowing untracked calls.

## Supported provider signals

During a search, built-in transports recognize:

| Signal | Meaning |
| --- | --- |
| `X-RateLimit-Remaining` + `X-RateLimit-Reset` | Remaining calls and reset Unix seconds |
| `RateLimit-Remaining` + `RateLimit-Reset` | Remaining calls and reset delta seconds |
| HTTP 429 + `Retry-After` | Exhausted until the delta seconds or HTTP date |
| Newznab/Torznab XML rate-limit error | Exhausted; reset unknown without an HTTP signal |

Remaining counts must be nonnegative integers no greater than 1,000,000,000.
Resets must be future timestamps within roughly one year. Unknown header
formats, non-scalar values and invalid remaining counts are ignored. A valid
remaining count with an invalid or absent reset still constrains admission;
the reset is unknown. HTTP 429 always means zero remaining, even if another
header claims a positive count. If both header families are present, Loom uses
the lower remaining count and later reset (an unknown reset stays unknown).

Absent signals leave the configured allowance in charge. Signals only tighten
the current provider allowance: concurrent/out-of-order responses cannot restore
consumed calls, and later reservations are subtracted from reported remaining
counts. At a known provider reset boundary the provider constraint expires;
the configured allowance still applies independently. A local reset never
clears an unexpired provider constraint.
Already admitted requests may still be in flight when a signal arrives;
the signal governs subsequent admissions, not requests the provider already saw.

**Unknown reset fails closed.** Exhaustion without a trustworthy reset has no
automatic resume time, even after a restart or configured-window boundary.
There is deliberately no “clear provider exhaustion” endpoint. Resolve the
provider's reset reporting before using the group again; changing local limits
alone cannot unlock it. Providers with unsupported reset formats may therefore
require operational intervention rather than automatic recovery.

## Skips and privileged changes

Exhausted providers produce `status: "skipped"` with a reason and `resume_at`
(Unix seconds when known) in aggregate diagnostics and streaming events.
Human-readable reasons include the UTC resume timestamp or explicitly say that
the provider reset is unknown. These skips do not trip or clear circuit breakers
or register as successful health samples.

Manual searches do not bypass provider allowances. There is no search-time
privileged override of provider-enforced limits. An administrator can raise a
**configured** allowance or change membership using `PUT`; this produces the
`indexer.budget_configured` audit event and preserves provider-reported state.
Only reassign groups when the real provider/account assignment changes:
detaching definitions or creating replacement groups discards that association
and must not be used as a way to evade a provider limit. Back up the database;
removing budget rows outside the API also removes their accounting history.
