# Native updates

## Scope and defaults

**Stable is the default. Automatic installation is off until an administrator
explicitly enables it.** Selecting Prelease additionally requires saving the
visible risk acknowledgement. Switching channels does not allow downgrades.

Implementation covers canonical **Linux/systemd and macOS/launchd, SQLite**
native services, with a matching root-owned `loom-updater` installed and enrolled.
Windows, PostgreSQL and unmanaged/manual installations remain manual-only.
Windows now has an isolated implementation candidate (SCM virtual account,
SYSTEM helper, protected startup event/job, NTFS transactions and restricted
SQLite snapshot child), **not production-enabled or shipped in normal archives**.
See [Windows candidate and activation blockers](windows.md#windows-updater-candidate-not-production-enabled).
Containers and Kubernetes continue using image/orchestration updates. PostgreSQL
can receive signed notices but cannot enable automatic installation.

**This is not yet a production-ready, all-platform feature.** The production
publisher public key is compiled from `internal/updates/trusted-keys.json`;
verified notices require matching signed manifests from the publisher.
Unit/transaction tests and an isolated Linux test of the actual
privilege-dropped WAL snapshot have passed. Native macOS tests cover descriptor
ACL inspection, authenticated startup IPC, process-image checks and a consistent
WAL snapshot, but **not privileged launchd enrollment, credential dropping,
upgrade/reboot recovery or an Intel runtime**. Live systemd recovery also remains
unvalidated. Keep production installations manual-only until the relevant native
acceptance tests pass and matching publisher releases are available.
Do not substitute test keys.

The Windows candidate has not run on a Windows kernel in this development
environment. In particular, its fail-closed process enumeration refuses any
unreadable process token (including protected processes); no evidence yet shows
that a stock Windows host can satisfy that quiescence check. SCM boot/crash
ordering, restricted-token WAL snapshots, full SACL-preserving replacement and
volume-flush/power-failure durability also need native acceptance. Cross-builds
and added Windows CI unit tests cannot establish those guarantees. The default
build deliberately rejects Windows enrollment.

## Linux installation boundary

Use the native archive's matching binaries and installer:

```sh
sudo sh packaging/linux/install.sh ./loom ./loom-updater
```

The second argument is optional; the one-binary workflow remains manual-only
for fresh installs. Existing enrolled helpers must be kept in sync when manually
upgrading. Enrollment is not consent to install: the application checkbox still
defaults to off. Enrollment requires a ready release binary with the correct
version, full commit SHA and source ref, not a development build.

The supported paths and service identity are fixed:

| Purpose | Binding |
|---|---|
| Main and helper executables | `/usr/local/lib/loom/{loom,loom-updater}`, root:root 0755 |
| Service | `loom.service`, user/group `loom`, canonical shipped systemd unit |
| Config / SQLite | `/etc/loom`, `/var/lib/loom/loom.db` |
| Config / data directory ownership | loom:loom 0750 |
| Runtime endpoint | `127.0.0.1:1925/api/v1/updates/native-runtime` |
| Private journal and rollback backups | `/var/lib/loom-updater`, root:root 0700 |

The helper validates root ownership, exact modes and canonical units, loaded
systemd properties, real PID 1, current process identity and effective SQLite
configuration. It refuses customized paths/units, hardlinks, symlinks and
extended attributes/ACLs rather than stripping or weakening them. This means
SELinux-labelled or otherwise customized installations may require manual
updates. Executable-directory permissions are never granted to the service.

There is no setuid binary, sudoers grant, generic command/path/URL API, or HTTP
apply endpoint. The root timer invokes a standalone helper. It independently
fetches and verifies metadata; the service supplies only authenticated
identity/readiness/preferences through a random-token endpoint. The token grants
no administrator access and is not included in logs. The helper has only fixed
run, enrollment, recovery and preflight operations, plus an internal snapshot
operation that refuses root and runs as the `loom` account.

## macOS installation boundary

The macOS archives include the matching helper for Intel and Apple Silicon:

```sh
sudo sh packaging/macos/install.sh ./loom ./loom-updater
```

Omitting the helper preserves manual installation on fresh installs. An already
enrolled installation requires both matching binaries for manual upgrades.
Enrollment requires a ready SQLite release build with source identity; it does
not enable automatic installation. Existing custom plists/permissions, ACLs,
xattrs and mutable-file flags are refused, not stripped or relaxed.

| Purpose | Binding |
|---|---|
| Executables | `/Library/Application Support/Loom/bin/{loom,loom-updater}`, root:wheel 0755 |
| Main service | `system/dev.loom.server`, dedicated `_loom:_loom` |
| Privileged helper | `system/dev.loom.updater`, root:wheel |
| Plists | `/Library/LaunchDaemons/dev.loom.{server,updater}.plist`, canonical root-owned 0644 |
| Config / SQLite | `/Library/Application Support/Loom/config`, `/Library/Application Support/Loom/data/loom.db` |
| Config / data directory ownership | _loom:_loom 0750 |
| Private journal, snapshots and helper logs | `/Library/Application Support/Loom/updater`, root-owned 0700 |
| Startup socket | `bin/updater.sock`, root:_loom 0660, protected root-owned parent |

The root helper is a persistent launchd daemon; it bounds each transaction and
checks hourly using persisted release-API backoff. Its Unix socket accepts only
the fixed read-only `START` request, with bounded clients, deadlines and request
sizes. Both sides authenticate native peer credentials: clients must be `_loom`,
the server must be root. No install request, command, path or URL can be supplied.

An enrolled main executable must receive a live startup authorization **before
loading/writing config or opening/migrating any database**. The gate starts
closed and remains closed until pending recovery is settled. This does not rely
on launchd job ordering. A missing/crashed helper blocks new starts rather than
allowing a candidate to migrate before rollback. The helper validates canonical
plist hashes, loaded main-job properties, launchd PID 1 and effective identity.
After bootout, **no process with the `_loom` UID may remain**, including escaped
descendants; otherwise it refuses snapshot/restore. Health additionally checks
the running process UID/GID and executable device/inode.

Pending journals block manual installation/uninstallation. Normally launchd
restarts the helper and recovery runs before authorizing startup. Inspect the
root-private helper logs on failure. If administrator intervention is necessary,
stop the helper job, run the installed helper's fixed `recover` operation, then
bootstrap its canonical plist again:

```sh
sudo launchctl bootout system/dev.loom.updater
sudo "/Library/Application Support/Loom/bin/loom-updater" recover
sudo launchctl bootstrap system /Library/LaunchDaemons/dev.loom.updater.plist
```

The standalone operation refuses an active helper lock and serves the startup
barrier while recovering. Do not delete the journal, remove enrollment markers,
change identities or bypass the gate to force startup. Uninstall preserves
private recovery state, config/data, logs and `_loom`. No Gatekeeper policy or
executable-directory ACL is relaxed.

## Windows: explicitly manual-only

Windows archives deliberately contain no updater helper. The installer continues
to run Loom as LocalService with read/execute-only access to the install directory;
only config/data/logs are service-writable. Signed notices remain possible once
matching signed manifests are published, but the API/UI cannot enable automatic installation.

An isolated candidate now implements these boundaries using a dedicated virtual
account, not shared LocalService: descriptor-preserving NTFS ZIP/`.exe`
transactions, restricted snapshot execution, protected event/job startup gating
and SCM restart/recovery ordering. It is not enabled in standard builds.
Elevated NTFS success/rollback/interruption tests are added to Windows CI, but
have only been cross-compiled here. Native SCM/token, container exclusion and
reboot/failure acceptance remain outstanding; protected-process quiescence is a
specific activation blocker. Cross-building does not validate those boundaries.
Do not work around this by running Loom as
LocalSystem, granting LocalService executable write access, or installing a
generic elevated task/command runner.

## Transaction and rollback

1. Validate enrollment, compiled trust-root fingerprint, current release identity,
   SQLite and saved opt-in. The Linux timer checks hourly with jitter; persisted
   metadata backoff survives helper restarts.
2. Verify the signed candidate and platform archive hash before stopping Loom.
   Never execute a downloaded binary as root or honor archive paths/ownership.
3. Stop and fully quiesce the service. Snapshot the entire config tree,
   preserving ownership/modes and refusing links/ACLs. Run SQLite `VACUUM INTO`
   as `loom` (Linux) or `_loom` (macOS), streaming a consistent snapshot into root-private
   storage; committed WAL data is included. Root does not parse the live
   service-controlled database through SQLite.
4. Validate the private snapshot, recheck consent and config, save the old
   executable and fsync snapshots before publishing a durable journal.
5. Atomically replace only the main executable, preserving its root ownership
   and mode. Start via systemd/launchd as the service identity; require authenticated version/SHA,
   SQLite readiness and the correct running executable inode.
6. On startup, migration or health failure, stop again and restore the old
   executable, complete config tree and database (removing new WAL/SHM files).
   Restart and health-check the old release. Remember the failed SHA to prevent
   repeated installation attempts.

The Linux boot recovery unit runs before Loom; macOS uses the live startup gate.
An interrupted update restores pre-upgrade state before database access;
unsuccessful recovery retains the
journal and blocks startup until retried. Do not delete the journal or backups
to bypass recovery. Installer/uninstaller refuse pending transactions.

Same-version Prelease updates additionally require the same signed branch and
a GitHub commit comparison proving a fast-forward from the enrolled SHA.
Root-owned baseline state prevents version downgrade/replay. The publisher
contract has **no expiry or monotonic publication counter**; it cannot prove
freshness against every stale-but-valid signed release on a fresh install.

Rollback is not an independent backup: media, remote systems and writes made
after a candidate starts are outside its scope. Keep off-host backups. Root-only
transaction directories are retained for diagnosis/recovery and currently have
no automatic retention cleanup; monitor disk usage.

## Exact publisher contract

GitHub Releases in `Ebenderooock/loom` supplies:

- `update-manifest.json`: the raw UTF-8 JSON bytes.
- `update-manifest.json.sig`: base64 of the raw 64-byte Ed25519 signature
  over **exactly those bytes**. No JSON signature envelope or domain prefix.

```json
{
  "schema_version": 1,
  "channel": "stable",
  "version": "0.4.2",
  "source_ref": "refs/tags/v0.4.2",
  "source_sha": "0123456789abcdef0123456789abcdef01234567",
  "assets": [
    {"platform": "linux-amd64", "name": "loom_0.4.2_linux_amd64.tar.gz", "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
  ]
}
```

Targets are `linux-amd64`, `linux-arm64`, `linux-armv7`, `darwin-amd64`,
`darwin-arm64`, `windows-amd64`, `windows-arm64`. Archives are `.tar.gz`,
or `.zip` for Windows. A manifest must contain exactly one archive for each
target. Unknown schema fields, invalid bindings, duplicate platforms/names and
untrusted signatures fail closed.

Stable uses the latest non-prerelease GitHub release and binds its tag to the
signed `refs/tags/v<version>`. Prelease selects the latest published non-draft
GitHub prerelease (within the newest 100 releases), with a signed
`refs/heads/prelease/X.Y.Z` (or `prerelease/` alias) matching the SemVer core.
Its unique GitHub release tag is **separate from the signed SemVer version**.
Only fixed HTTPS GitHub API/release URLs and GitHub asset redirects are used.

UI checks are demand-driven, not startup/background polling. Concurrent checks
share a per-channel one-hour cache, including errors; ETags refresh metadata.
GitHub rate-limit delays are honored up to 24 hours. Missing trust roots make
no network requests. Checks download only metadata, never a binary.

Notification connections can opt in separately to an instance-updated alert
and an update-available alert. The first silently records the running version
on first startup and alerts on subsequent version changes, regardless of how
Loom was updated.
The second alerts once per version when a demand-driven check finds a newer,
verified release for the selected channel. It requires the compiled publisher
trust root and matching signed manifests; unsigned GitHub metadata is not used.
The version history is stored in the database. Delivery uses the existing
best-effort notification system.

## Remaining isolated publisher work

Source CI builds seven native Prelease targets as unsigned Actions artifacts;
Linux/macOS archives include `loom-updater`; Windows archives do not. The inventory is build input, **not** a
signed update manifest. No source workflow receives signing/publishing secrets.

The separate `Ebenderooock/loom-publisher` repository must:

- Publish **both Stable and Prelease** native assets and the exact manifest/signature.
- Independently verify source ref/SHA and archive identity; embed the full commit
  and matching source ref in main binaries (`internal/buildinfo.SourceRef`).
- Package matching Linux/macOS helpers and canonical service files. The main and
  helper must contain the same owner-provisioned public trust roots.
- Keep the private signing key exclusively in the isolated publisher. Support
  public key rotation through reviewed source changes with overlapping keys.
- Validate end-to-end live systemd/launchd enrollment, update, rollback and
  interrupted-boot paths before enabling production enrollment.

No release or asset was published by this implementation.
