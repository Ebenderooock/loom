# Windows: native service installation

Loom can run as a Windows service with no third-party service wrapper. The
release ZIP must contain `loom.exe`, `packaging/windows/install.ps1` and
`packaging/windows/uninstall.ps1`, preserving these paths. This is a script
installer, **not an MSI or a signed executable**. These instructions
do not imply that a Windows asset has already been published: use a release
that actually lists it, or build the service-aware binary from this source.
An older binary without the `service` command will not work as a service.

## Download and install

1. Visit [Loom releases](https://github.com/Ebenderooock/loom/releases).
   Select a Windows `amd64` ZIP for x64 Windows, or `arm64` for Windows on ARM.
   Windows ARMv7 is not supported. Use a currently supported Windows version.
2. Download the ZIP and the release's `checksums.txt`. Compare
   `(Get-FileHash .\YOUR_ARCHIVE.zip -Algorithm SHA256).Hash` with the line
   for that **exact** archive in `checksums.txt`. A checksum is an integrity
   check, not a publisher signature.
3. Extract to a staging directory, **not** to Program Files or ProgramData.
   Review the scripts before running them. Open **PowerShell as Administrator**,
   change into the extracted archive root, then run:

```powershell
# If downloaded files are blocked, unblock only the scripts you have reviewed.
Unblock-File .\packaging\windows\install.ps1
Unblock-File .\packaging\windows\uninstall.ps1
& .\packaging\windows\install.ps1
# An explicit source is also supported:
# & .\packaging\windows\install.ps1 -BinaryPath .\loom.exe
Get-Service Loom
& "$env:ProgramFiles\Loom\loom.exe" healthcheck
```

Windows PowerShell 5.1 and PowerShell 7 are supported. If your execution policy
prevents running a reviewed unsigned script, use a process-scoped policy in
that elevated shell, subject to your organization's policy:
`Set-ExecutionPolicy -Scope Process -ExecutionPolicy RemoteSigned`.
Do not disable machine-wide execution policy or bypass organizational controls.

Open **http://127.0.0.1:1925** and complete first-run setup. No firewall rule
is created. Startup is automatic (delayed) after reboot. The installer runs
`version` to check loader/architecture compatibility before stopping an old
service. It does not authenticate arbitrary input binaries: verify downloads.

## Identity, storage and access

| Item | Installed location |
|---|---|
| Executable | `%ProgramFiles%\Loom\loom.exe` |
| Config | `%ProgramData%\Loom\config\loom.yaml` |
| Application settings/secrets | `%ProgramData%\Loom\config\loom.json` |
| Data/definitions | `%ProgramData%\Loom\data` |
| SQLite | `%ProgramData%\Loom\data\loom.db` |
| Combined stdout/stderr logs | `%ProgramData%\Loom\logs\loom.log` |

The service is named **Loom** and runs as **NT AUTHORITY\LocalService**, not
LocalSystem. Administrators and SYSTEM have full directory access; LocalService
has read/execute on the binary directory and modify on config/data/logs.
Other built-in LocalService processes share this identity; this is not
per-service isolation. The executable directory is not writable by the service.
Do not store your media in Program Files.

Grant LocalService read/write permissions only on the download and library
directories Loom must manage. Mapped drive letters in your login session are
not visible to services. LocalService does not supply your credentials to NAS
shares; use local storage, or arrange a dedicated service account with suitable
share/NTFS permissions. Re-running this installer resets the account to
LocalService; custom-account deployments need manual service management.
Remote mounts must be available before import tasks run.

All three directories and the SQLite path are set explicitly in YAML.
`--config` selects the file; it does not itself set `config_dir` or `data_dir`.
Loom's ordinary defaults are `./config` for both directories, SQLite at
`<config_dir>/data/loom.db`, and HTTP `:1925` (all interfaces). The installer
intentionally overrides those defaults with absolute paths and loopback HTTP.
Machine/service `LOOM_*` environment settings can override YAML; check them
when troubleshooting unexpected storage or listen addresses.

For LAN use, complete setup first, stop the service, change `http.addr` in
`loom.yaml` to a specific LAN address or `:1925`, restart, and restrict inbound
TCP 1925 to trusted networks in Windows Firewall. For internet access, use a
TLS reverse proxy and appropriate authentication; do not directly expose 1925.

## Lifecycle, upgrades and removal

```powershell
Stop-Service Loom
Start-Service Loom
Get-Content "$env:ProgramData\Loom\logs\loom.log" -Tail 100 -Wait
```

SCM invokes `loom.exe service --config ... --log-file ...`, **not console
`serve` via `sc.exe`**. Stop/Shutdown cancels the same server context as console
SIGTERM; HTTP has a 30-second shutdown budget and other components also perform
cleanup. SCM receives StopPending checkpoints until cleanup returns. Windows
may impose a shorter budget during OS shutdown; explicitly stop the service
before maintenance/reboots involving storage.

SCM **Running is not HTTP readiness**: initialization/migrations can still be
in progress. Retry `healthcheck` and inspect logs. Worker failures report
service-specific exit code 1 and recovery restarts after 10/30/60 seconds.
Config/log-open failures before the dispatcher also fail startup; consult
Event Viewer → Windows Logs → System (Service Control Manager) for those.

Before upgrading, stop Loom and back up the entire config/data directories
(including SQLite sidecars if present). Do not copy a live SQLite file in
isolation. Download/verify/extract the new release, then re-run its installer;
it stops the old service, keeps `loom.previous.exe`, preserves YAML/JSON/data,
and starts the new binary. Check health/logs before deleting the staging files.
If migrations have run, an old binary may not understand the new database:
rollback may require restoring the **matching stopped config/data backup**,
not just replacing the executable.

Logs append across restarts with no automatic file rotation. Stop Loom before
archiving/rotating `loom.log`, then start it again; monitor disk usage.

To uninstall, run the extracted `packaging/windows/uninstall.ps1` as
Administrator. It stops/deletes the managed service and installed executables,
but **preserves ProgramData, logs and media**. Close Services/Event Viewer if
Windows marks deletion pending; reboot before reinstalling if necessary.
Inspect and back up preserved paths before manually removing them.

## Updates: manual installation only

Windows does **not** ship a privileged auto-update helper. Signed Stable/Prelease
notices can be shown when matching signed manifests are published, but selecting a
channel does not enable installation. Use a freshly extracted matching release
and this installer for manual upgrades; retain a stopped config/database backup
when migrations may run.

The isolated implementation candidate below is not production-enabled.
Cross-compilation alone does not establish its guarantees.
LocalService remains read/execute-only on
the installation directory; do not grant it Modify access, run Loom as
LocalSystem, or substitute a generic privileged scheduled task.

## Windows updater candidate (not production-enabled)

Source now contains a Windows-specific transaction pipeline; it is **not a
supported auto-update release**. Standard Windows binaries fail closed and
ordinary release/Prelease ZIPs still omit the helper. The separate
`.goreleaser.windows-updater-candidate.yaml` builds matching amd64/arm64 pairs
with `loom_updater_windows_candidate`, names archives `UNVALIDATED`, and disables
release publication. Supply `CANDIDATE_VERSION` and `CANDIDATE_REF` matching a
real release/Prelease source identity; never send these archives to the
publisher. Do not remove the production activation barrier on cross-build or
unit-test evidence alone.

Candidate enrollment is explicit, elevated, and **disposable VM only**:

```powershell
& .\packaging\windows\install.ps1 -BinaryPath .\loom.exe -UpdaterPath .\loom-updater.exe
```

Only a fresh fixed-layout installation or an already matching virtual-account
installation is accepted. Existing LocalService/custom services are refused,
not silently converted: a shared SID cannot safely attribute escaped writers.
Keep existing installations manual-only. The main account is `NT SERVICE\Loom`,
with only traverse privilege requested from SCM; `LoomUpdater` is LocalSystem.
Both executables and their parent are SYSTEM/Administrators-owned and
read/execute-only for Loom. The ProgramData base is also read-only to Loom;
only config/data/log subdirectories grant it Modify. Private updater state has
no service ACE. Paths come from OS known-folder APIs, not environment overrides.

The service wrapper waits before config/log/database access. Its fixed global
event and no-breakaway job are authenticated by protected descriptors plus the
live SCM helper's image/account. There is no caller-command/path/URL API.
Maintenance uses one fixed SCM control requiring an administrator-only right.
The helper independently verifies the same signed release manifest as Unix,
disables SCM restarts during stop, requires an empty job and no remaining
dedicated-account processes, and snapshots SQLite through a privilege-stripped
captured service token. The snapshot process starts suspended and is contained
before resumption. Root/SYSTEM never opens the service-controlled source DB
through SQLite.

The separate NTFS pipeline captures owner/group/DACL/SACL, refuses hardlinks,
reparse points, alternate streams, EAs and unsupported attributes, preserves the
descriptor on replacement, and uses write-through same-volume replacement plus
volume flushes rather than Unix directory-fsync assumptions. A private durable
journal gates replacement; restoration includes full config, SQLite, sidecar
removal, the old executable and failed-build blocking. Startup recovery precedes
database authorization. Unsupported ACLs/filesystems/configurations fail closed.
Transaction staging and configuration-backup directories are created with an
explicit SYSTEM owner and protected private DACL, independent of the creating
token's default owner.

**Exact activation blockers:** there is no Windows execution host available in
this development environment. Native acceptance must prove SCM dependency,
crash/reboot and queued-restart ordering; virtual-account token creation and
restricted WAL snapshots; NTFS SACL/stream/EA handling and volume-flush durability;
successful upgrade, migration/health rollback and interrupted recovery. In
addition, quiescence currently refuses every unreadable process token, including
protected Windows processes: establish whether this is satisfiable on supported
Windows versions, or design a sound alternative before activation. Do not
silently ignore access-denied processes. Added Windows CI tests cover pure/native API boundaries
and isolated elevated NTFS success, migration/health rollback and interrupted
recovery with a fake service controller. They are cross-compiled, **not executed
here**, and do not test real SCM enrollment, restricted-token creation, container
exclusion or reboot/power-loss acceptance.

After those blockers are resolved, validate on disposable supported Windows
hosts for both architectures using matching signed release archives: enroll the
candidate, opt in as an administrator, upgrade a database with committed WAL
data, and verify the installed version and health. Force a migration or health
failure and verify restoration of the previous executable, configuration and
database, including removal of candidate sidecars. Interrupt each journal phase
and reboot; verify recovery finishes before the service opens the database.
Verify that post-rollback writes survive a later recovery attempt and that a
committed candidate is not reverted after interruption. Writes made by a
candidate before its durable commit can still be lost on rollback; solve or
explicitly accept that limitation before activation.
Exercise SCM crash/restart ordering, access-denied process handling, protected
ACLs, unsupported filesystems and container exclusion. Record the Windows
versions, architecture and results; retain independent backups.

Only after that evidence is reviewed should a separate production change remove
the `supportedNative = windowsCandidate` barrier in
`internal/updater/paths_windows.go`, include matching helpers and enrollment in
normal Windows release archives, and update the API and documentation to match.
The candidate build tag alone is for acceptance testing, not production
activation.

For candidate recovery, retain both matching binaries and ProgramData state.
After a crash, SCM restarts the helper, which replays a pending journal before
opening the gate. If that fails, leave Loom stopped and preserve
`ProgramData\Loom\updater` for diagnosis; never delete the journal or start a
different binary against partially restored data. Restore a matching stopped
config/database backup administratively if automated recovery cannot succeed.
Installer upgrades and uninstall refuse pending recovery; uninstall retains
config/data/private snapshots. Do not overwrite an installed helper while its
service is active.

## Native smoke test (Windows CI or a disposable VM)

Run `go test -race ./cmd/loom` on Windows to execute the SCM handler tests.
The bounded hosted-runner test is also provided (elevated PowerShell, **fresh
ephemeral runner only**):

```powershell
$env:CGO_ENABLED = '0'
go build -o .\loom-test.exe ./cmd/loom
if ($LASTEXITCODE -ne 0) { throw 'Build failed' }
& .\packaging\windows\smoke-test.ps1 -BinaryPath .\loom-test.exe
```

Give this CI job a 10-minute timeout. It chooses an isolated loopback port,
polls `/readyz` with a 90-second bound, checks LocalService/SCM command line,
stop/start, upgrade with preserved custom config/data/SQLite paths, and
data-preserving uninstall. Its `finally` attempts service deletion even on
failure and removes **only its newly created test installation** after verifying
the service is absent. It refuses pre-existing service/data/binary directories.
It does not test reboot behavior, signatures, or the embedded web UI.

For installer acceptance, use a disposable host with no pre-existing Loom:
install a verified release; inspect `sc.exe qc Loom` for LocalService and
quoted absolute paths; retry healthcheck; create first-run settings; stop/start;
reboot and check delayed startup; upgrade from a second staging directory;
verify unchanged config/data and appended logs; uninstall; verify service
absence and retained SQLite/config. Include a path containing spaces and a
non-English Windows locale. Never run service installers on a shared CI host.
