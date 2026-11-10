# Linux: native systemd installation

Use a locally downloaded Loom binary with the supplied service installer.
This path requires **systemd**, root/sudo, and `useradd`, `groupadd`, `getent`
(normally supplied by your distribution's account-management packages).
It is not a `.deb` or `.rpm`. For non-systemd systems, run `loom serve --config
/absolute/path/loom.yaml` under your distribution's supervisor, or use Docker.

## Download and install

Choose a Linux release at [Loom releases](https://github.com/Ebenderooock/loom/releases):

| `uname -m` / OS | Binary architecture |
|---|---|
| `x86_64` | `linux_amd64` |
| `aarch64` on a 64-bit OS | `linux_arm64` |
| ARMv7 with a 32-bit OS | `linux_armv7` |

ARMv6 is unsupported. See [Raspberry Pi](raspberry-pi.md) for OS/CPU selection.
Use the archive name actually listed for your chosen release, not a guessed
unpublished version. Download its `checksums.txt` and verify the exact archive
before extraction (`sha256sum --ignore-missing -c checksums.txt`; ensure your
archive is reported **OK**, not merely skipped).

The extracted archive should contain `loom`, `packaging/linux/` and `docs/`,
preserving these paths. From its root:

```sh
chmod +x ./loom
sudo sh packaging/linux/install.sh ./loom
systemctl status loom.service --no-pager
journalctl -u loom.service -n 100 --no-pager
/usr/local/lib/loom/loom healthcheck
```

Review the scripts before running them. The installer executes the supplied
binary's `version` as root to check loader compatibility; it does not establish
publisher trust. The root-owned binary is installed at
`/usr/local/lib/loom/loom` and the enabled unit at
`/etc/systemd/system/loom.service`. Open **http://127.0.0.1:1925** and complete
setup. For a headless host, tunnel from your workstation:
`ssh -L 1925:127.0.0.1:1925 user@host`, then open that same local URL.

## Identity, paths and media permissions

The dedicated non-login **loom:loom** account runs the service.

| Item | Location |
|---|---|
| YAML config | `/etc/loom/loom.yaml` |
| App settings/secrets | `/etc/loom/loom.json` |
| Data/definitions | `/var/lib/loom` |
| SQLite | `/var/lib/loom/loom.db` |
| Logs | systemd journal: `journalctl -u loom` |

Config/data directories are mode 0750, owned by loom; initial YAML is 0640.
Service-created files use umask 0027. The service must be able to update
`loom.json`, so config is writable by loom. Executables/unit are root-owned.
An existing account with a different home or an unmanaged unit is refused:
back up/migrate an old installation manually, do not run two Loom processes
against the same SQLite database.

The supplied YAML explicitly sets `config_dir`, `data_dir`, SQLite path and
loopback HTTP. Ordinary Loom defaults are `./config` for config/data,
`<config_dir>/data/loom.db` for SQLite, and `:1925` (all interfaces).
Changing only `--config` does **not** relocate the other paths. `LOOM_*`
environment overrides take precedence over YAML.

Grant loom access to your specific library/download directories, for example
through an existing shared media group: `sudo usermod -aG media loom`, then
restart the service. Ensure parent directories permit traversal and NAS
mounts grant the appropriate numeric UID/GID access. Do not recursively
`chmod 777` or blindly change ownership of an existing library.
`PrivateTmp=true` means the service does not share the host's `/tmp`; use
dedicated download directories instead.

For mount dependencies use `sudo systemctl edit loom`:

```ini
[Unit]
RequiresMountsFor=/srv/media /srv/downloads
```

Then `sudo systemctl daemon-reload && sudo systemctl restart loom`.
Do not start imports before the mounts are ready.

For LAN use, complete setup via loopback/tunnel first; stop Loom, set
`http.addr` in YAML to a specific LAN address or `:1925`, restart, and restrict
TCP 1925 using your firewall. Internet access needs a TLS reverse proxy and
appropriate authentication; don't publish 1925 directly.

## Operations, upgrades and uninstall

```sh
sudo systemctl stop loom
sudo systemctl start loom
journalctl -u loom -f
```

Stop sends SIGTERM. Loom budgets 30 seconds for HTTP shutdown, then completes
other cleanup; the unit allows 120 seconds before forced termination.
`Type=simple` reports a running process, not completed initialization: retry
healthcheck and check the journal. Failure restarts are rate-limited.
Journal retention/rotation is controlled by the host's journald configuration;
configure disk limits there. In-app captured logs do not replace the journal.

Before upgrading, stop the service and back up **both `/etc/loom` and
`/var/lib/loom`**, including any SQLite sidecars. Verify/extract the new release
to a staging directory and rerun `sudo sh packaging/linux/install.sh ./loom`.
The old executable is retained as `/usr/local/lib/loom/loom.previous`; existing
config/data are not overwritten or recursively re-owned. Inspect health/logs.
If an upgrade fails, stop Loom; restore the old executable and the matching
stopped config/data backup if schema migrations occurred; restart. Restoring
only an old executable is not a safe database downgrade.

Run `sudo sh packaging/linux/uninstall.sh` from the extracted archive to stop,
disable and remove the managed unit and binaries. **Config, data, journal,
service account and all media are preserved.** Remove them manually only after
inspection and backup. Reinstalling reuses preserved data.

For native acceptance testing in a disposable systemd VM: install, check unit
identity and permissions, retry healthcheck, reboot, stop/start, upgrade from a
fresh staging directory with unchanged config/data, then uninstall and verify
data retention. Do not install services on a shared build host.
