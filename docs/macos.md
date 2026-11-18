# macOS: native launchd installation

Loom runs as a system LaunchDaemon, starting at boot without a logged-in user.
The installer is a shell script, **not a signed/notarized `.pkg` or app bundle**.
Use a release that actually contains the macOS archive and packaging files.

## Download and install

Visit [Loom releases](https://github.com/Ebenderooock/loom/releases), choosing
`darwin_arm64` for Apple Silicon or `darwin_amd64` for Intel. Verify the exact
archive's SHA-256 against the release's `checksums.txt` using
`shasum -a 256 YOUR_ARCHIVE.tar.gz`. Extract to a staging directory. The archive
must preserve `packaging/macos/` and `docs/` with `loom` at its root.
Review the scripts, then from the extracted root:

```sh
chmod +x ./loom
./loom version
sudo sh packaging/macos/install.sh ./loom
sudo launchctl print system/dev.loom.server
sudo "/Library/Application Support/Loom/bin/loom" healthcheck
```

If macOS blocks the unsigned downloaded executable, verify its origin/checksum
and use your organization's approved Gatekeeper exception process. For a file
you have reviewed and trust, remove **only that file's** quarantine attribute
with `xattr -d com.apple.quarantine ./loom` if present. Do not disable Gatekeeper
globally. The installer tests `version` before stopping an existing service;
it cannot authenticate an arbitrary supplied executable.

Open **http://127.0.0.1:1925** and complete first-run setup.

## Optional privileged updater (production activation pending)

For the canonical SQLite service only, the matching `loom-updater` can be
enrolled by supplying it as the installer's second argument:

```sh
sudo sh packaging/macos/install.sh ./loom ./loom-updater
```

This installs a separate root launchd daemon, not a writable executable directory
or a sudoers rule. The main service remains `_loom`. Automatic installation stays
off until separate administrator consent; Stable remains the default and Prelease
requires an additional risk acknowledgement. PostgreSQL and custom layouts stay
manual-only. Once enrolled, manual upgrades require **both** matching binaries.

The enrolled main process waits on a root-authenticated startup barrier before
config/database access, including crash/reboot starts. Missing helper or unsettled
recovery means startup fails closed. Never remove the marker/journal to bypass it.
Private helper logs and retained snapshots live under
`/Library/Application Support/Loom/updater`, root-owned 0700; inspect using `sudo`.
The helper job is `system/dev.loom.updater`.

This source build has no production signing public key, so actual notice
verification/automatic installation remain unavailable. Native unprivileged
security/transaction tests are not privileged launchd acceptance testing.
Validate enrollment, stopped-service credential-dropped snapshots, successful
upgrade, migration/health failure rollback, helper termination during every
journal phase, reboot-before-recovery, escaped `_loom` descendants, custom
ACL/xattr refusal, and recovery-preserving uninstall on disposable Apple Silicon
and Intel systems before production activation. See
[native updates](native-updates.md) for the exact boundary and recovery commands.

## Identity, storage and permissions

| Item | Location |
|---|---|
| Binary | `/Library/Application Support/Loom/bin/loom` |
| YAML | `/Library/Application Support/Loom/config/loom.yaml` |
| App settings/secrets | `/Library/Application Support/Loom/config/loom.json` |
| Data/definitions | `/Library/Application Support/Loom/data` |
| SQLite | `/Library/Application Support/Loom/data/loom.db` |
| stdout log | `/Library/Logs/Loom/loom.log` |
| stderr/startup errors | `/Library/Logs/Loom/loom-error.log` |
| LaunchDaemon | `/Library/LaunchDaemons/dev.loom.server.plist` |

The installer creates a dedicated hidden non-login **_loom** user/group, picking
an unused system ID in 400–499. It refuses incompatible existing identities or
an unmanaged daemon. The binary/plist are root-owned; config/data/log
directories are _loom-owned mode 0750, initial YAML 0640, and the service umask
is 0027. Config is writable because Loom updates `loom.json`.

Absolute config/data/SQLite paths and loopback HTTP are explicit in YAML.
Ordinary Loom defaults are `./config` for both directories, SQLite at
`<config_dir>/data/loom.db`, and HTTP `:1925` (all interfaces). `--config`
selects YAML but does not relocate the other directories. Service `LOOM_*`
environment variables override YAML.

Grant _loom read/write access only to the required download/library folders
and traversal access to their parents, using a shared media group or narrowly
scoped macOS ACLs. Finder-mounted SMB volumes and login keychains are not
automatically available to a boot daemon. Configure persistent system mounts
and permissions before enabling imports. macOS privacy controls (TCC) may
also deny access to protected user folders; prefer dedicated media storage
outside Desktop/Documents rather than assuming a daemon inherits your access.
Do not grant Full Disk Access or broad write access unless actually required.

For LAN access, complete setup first, stop the daemon, change `http.addr` to a
specific LAN IP or `:1925`, and restart. Restrict access to trusted networks;
the installer does not create firewall exceptions. Internet access requires a
TLS reverse proxy and appropriate authentication, not direct port exposure.

## Operations, upgrade and removal

```sh
# Clean stop (SIGTERM); do not use kickstart -k for ordinary maintenance.
sudo launchctl bootout system/dev.loom.server
# Start again:
sudo launchctl bootstrap system /Library/LaunchDaemons/dev.loom.server.plist
sudo launchctl print system/dev.loom.server
sudo tail -n 100 /Library/Logs/Loom/loom.log /Library/Logs/Loom/loom-error.log
```

The daemon runs at load and restarts on non-zero exit with a 10-second
throttle. `ExitTimeOut=120` allows cleanup before launchd forcibly kills it;
HTTP shutdown has a 30-second budget plus other component cleanup. A loaded
job is not HTTP readiness: retry healthcheck and inspect logs.

Before upgrading, bootout and back up **config and data**, including SQLite
sidecars. Verify/extract the new archive and rerun its installer. It bootouts
any loaded job, retains `bin/loom.previous`, preserves config/data, and
bootstraps the new daemon. Inspect health/logs. To roll back, bootout, restore
the previous executable **and the matching stopped config/data backup if
migrations ran**, then bootstrap again. Old executables may not support new
database schemas.

File logs append across restarts; there is no automatic installer-managed
rotation. Monitor their size. Bootout before rotating/archiving both log
files, then bootstrap to reopen them.

Run `sudo sh packaging/macos/uninstall.sh` from the extracted archive. The
script bootouts/removes the managed daemon and binaries but **preserves config,
data, private updater recovery state, logs, _loom user/group and media**. Pending
update transactions must be recovered first. Inspect/back up before manually deleting
retained paths or accounts. Reinstalling reuses them.

For native acceptance testing on a disposable Mac/VM: install, verify _loom
identity, plist lint/ownership and path permissions, retry healthcheck, reboot
without login, stop/start, upgrade with unchanged config/data, uninstall and
verify retention. Test Apple Silicon and Intel separately; don't install a
daemon on a shared build machine.
