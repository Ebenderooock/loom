# ADR-0030: Separate request fulfillment from storage observations

- Status: Accepted
- Date: 2026-10-24
- Deciders: @loom-maintainers

## Context

Deleting media must not erase who requested it, yet old fulfillment cannot
permanently prevent another authorized acquisition. Catalog membership, grabs,
monitoring, file presence and external-server playback are different facts.
An unmounted filesystem can leave an empty directory, making absence inferred
from scan records unsafe.

## Decision

Keep `available` as a backward-compatible historical fulfillment state with an
immutable first-fulfillment timestamp. Store current present/missing/unknown
observations and verified path evidence independently. Require a matching
storage-root marker and previously verified file evidence before confirming
absence. Explicit re-requests append linked pending requests, preserve history,
enforce one global active request and consume normal per-user rolling quotas.
Reconcile non-destructively with dry runs and atomic request-update/audit batches.
Keep monitoring and unimplemented media-server observations independent.

## Consequences

### Positive

- File/catalog deletion and mount loss cannot rewrite fulfillment history.
- Empty disconnected roots cannot authorize bulk missing or re-acquisition.
- Batch previews and per-item audit outcomes make reconciliation inspectable.

### Negative / trade-offs

- Initial marker registration needs write access to the library root.
- Migrated already-missing files lack verified evidence and remain unknown.
- Nested storage volumes need separate library roots; interrupted registration
  or deliberately relocated roots require explicit operator inspection.
- Filesystem marker registration cannot participate in the database transaction.

### Neutral

- `available` remains the wire value; UI/bot descriptions say historical fulfillment.
- External media-server presence stays unknown until an observation source exists.
- Fulfilled history continues to count against quotas inside its original window.

## Alternatives considered

- Reopening or deleting old requests loses request/fulfillment history.
- Treating stale file records as presence or deletion confuses scans with facts.
- Checking only root-directory existence cannot detect disconnected mountpoints.
- Automatically trusting empty roots would undermine the storage safety boundary.
