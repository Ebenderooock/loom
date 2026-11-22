# ADR-0035: Restricted opt-in native updates

- Status: Accepted; bounded Linux/macOS implementation, production activation pending
- Date: 2026-11-17
- Deciders: @Ebenderooock

## Context

Native updates must not make the service account able to replace its executable.
Publishing authority belongs to the isolated publisher (ADR-0034). Copying live
SQLite files with the existing backup command cannot guarantee migration-safe
rollback.

## Decision

Use GitHub Releases and the publisher's raw UTF-8 `update-manifest.json` plus
base64 detached Ed25519 signature over exactly its bytes. There is no signature
envelope, domain prefix or expiry field. Trust roots are compiled public keys;
private keys never enter this repository. Signed source refs/SHA bind channels;
unique Prelease release tags are independent of their SemVer version.

Stable is the default. Auto-install is off until administrator consent;
Prelease needs a separate visible acknowledgement. Support automatic installation
only for installer-managed canonical Linux/systemd and macOS/launchd SQLite services, through
a dedicated root helper with fixed paths/operations and no generic execution
surface. Preserve binary ownership/ACLs; refuse customized or unsafe layouts.
Windows, PostgreSQL, manual installs and containers remain fail-closed.

Windows has a separately gated, unvalidated implementation candidate: a dedicated
`NT SERVICE\Loom` virtual account, SYSTEM SCM helper, protected fixed event/job
startup barrier, privilege-stripped snapshot child and separate NTFS
descriptor-preserving transaction/recovery engine. Never convert an existing
shared-LocalService installation silently. Only isolated candidate builds carry
matching Windows helpers; ordinary releases cannot enroll. Candidate compilation
does not authorize activation.

macOS uses a root launchd helper and fixed read-only, peer-credential-authenticated
Unix startup barrier before any config/database access. The gate closes before
stop and is reopened only after recovery or a controlled candidate/restored
start. Quiescence requires no remaining `_loom` UID processes, not merely a
stopped launchd job. Inspect ACLs on the open vnode through `fgetattrlist`;
`ls -e /dev/fd/N` misses native ACLs. Unsupported kernels/metadata fail closed.

Stop and quiesce Loom before consistent SQLite/config snapshots. Parse live
SQLite only in a credential-dropped child, validate the root-private snapshot,
and journal before replacement. Authenticate version-specific health, roll back
binary/config/database on failure and recover interrupted transactions before
booting Loom. Keep root-owned anti-downgrade state and block failed SHAs.

## Consequences

- The helpers, Linux installer/timer, macOS launchd enrollment/startup barrier,
  shared snapshot/rollback engine and focused tests
  are implemented; this is not complete multi-platform production support.
- A production publisher public key is provisioned. Real notice verification
  requires matching published signed manifests; live native validation remains
  necessary before production activation of automatic installation.
  macOS tests currently cover unprivileged boundaries, not root launchd,
  credential dropping or reboot recovery. Windows candidate code/tests have
  only been cross-compiled here; actual SCM/token/reboot acceptance, container
  exclusion and protected-process quiescence remain unproven. Default Windows
  archives ship no helper and their API remains manual-only.
- The publisher still needs Stable and Prelease manifests and matching native
  binary/helper builds. Source CI carries no signing or publishing authority.
- No signed expiry/counter means freshness/replay protection has limits;
  root-owned installed-state and fast-forward checks are not a replacement.
- Rollback is not an independent backup; retained private snapshots need disk
  monitoring. See [native updates](../native-updates.md) for precise limits.
