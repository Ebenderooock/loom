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
data, logs, _loom user/group and media**. Inspect/back up before manually deleting
retained paths or accounts. Reinstalling reuses them.

For native acceptance testing on a disposable Mac/VM: install, verify _loom
identity, plist lint/ownership and path permissions, retry healthcheck, reboot
without login, stop/start, upgrade with unchanged config/data, uninstall and
verify retention. Test Apple Silicon and Intel separately; don't install a
daemon on a shared build machine.
