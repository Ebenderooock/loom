# ADR-0036: PostgreSQL native update candidate

- Status: Proposed; production activation blocked
- Date: 2026-11-23
- Deciders: @ebenderoock

## Context

PostgreSQL supports shared and HA installations. Stopping a local Loom service
does not stop other clients from changing its database. Rolling back a migration
against such a database can destroy unrelated writes.

## Decision

Keep PostgreSQL auto-install disabled in production while developing an isolated
candidate. A future implementation must require explicit administrator
confirmation that the database is dedicated to Loom, bound to the effective
host, port, database and user. A changed identity invalidates that confirmation.
Shared, HA, split-mode and remote installations remain manual-only.

The candidate must stop and quiesce Loom, fence other database clients, capture
and validate a custom-format `pg_dump`, and durably journal all recovery state
before replacement. Recovery must recreate the database with its original
owner, encoding and locale before restoring the dump, so migration-created
objects do not survive. It must restore any connection restrictions after a
crash. Commands must use trusted fixed paths, compatible PostgreSQL major
versions and credential-dropped execution without passwords in arguments.

The checked-in candidate is not an end-to-end updater. Its identity and
snapshot orchestration are isolated behind the `loom_postgres_candidate` build
tag; the tools, durable fence, database recreation, restore and recovery
integration do not yet exist. Administrators can record an acknowledgement
against the current identity, but the record is stored in that database:
switching to another database and then back can revive the old record. An
independent durable binding that invalidates consent across databases is
required before it can authorize installation. A proposed solution pairs a
root-owned host-local identity and random nonce with a matching database record,
invalidating the host record when the identity changes. The service cannot
write the root-owned state directly; the authorization and crash-safe protocol
for updating it remain to be designed.

`ALLOW_CONNECTIONS false` also blocks new connections by `pg_dump` and
`pg_restore`. A connection limit does not fence privileged clients. Neither is
an updater-only fence on its own. A design that guarantees updater-only access,
including across crashes, is required before activation; a session-count check
alone is not a fence.

## Consequences

- PostgreSQL installations continue to receive notices but cannot enable
  automatic installation in ordinary or candidate builds.
- No live systemd or launchd PostgreSQL upgrade, rollback or crash recovery has
  been validated. Native acceptance and an independent backup remain necessary.
- Fake tests exercise candidate preflight and snapshot orchestration, not a real
  PostgreSQL server or a working rollback.

## Alternatives considered

- Restoring with `pg_restore --clean` alone can leave objects created by newer
  migrations and does not provide complete rollback.
- Trusting a zero-session observation without a fence permits a new client to
  connect immediately afterward.
