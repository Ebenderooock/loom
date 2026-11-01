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
