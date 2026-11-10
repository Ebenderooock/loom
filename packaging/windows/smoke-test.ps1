#Requires -Version 5.1
#Requires -RunAsAdministrator
<#
Bounded SCM/installer integration check for an ephemeral Windows hosted runner.
Not for a developer's workstation: deliberately removes its own test files.
Build a service-aware binary first; no embedded web UI is required for /readyz.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$BinaryPath
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$BinaryPath = (Resolve-Path -LiteralPath $BinaryPath).ProviderPath
$base = Join-Path $env:ProgramData 'Loom'
$installDir = Join-Path $env:ProgramFiles 'Loom'
# Refuse to touch an existing installation, including preserved user data.
if ((Get-Service -Name Loom -ErrorAction SilentlyContinue) -or
    (Test-Path -LiteralPath $base) -or (Test-Path -LiteralPath $installDir)) {
    throw 'SCM smoke test requires a disposable runner without Loom service/data/binaries.'
}
$listener = New-Object System.Net.Sockets.TcpListener([System.Net.IPAddress]::Loopback, 0)
$listener.Start()
$port = $listener.LocalEndpoint.Port
$listener.Stop()
$url = "http://127.0.0.1:$port/readyz"

function Wait-LoomReady {
    $deadline = [DateTime]::UtcNow.AddSeconds(90)
    while ([DateTime]::UtcNow -lt $deadline) {
        try {
            $response = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 3
            if ($response.StatusCode -eq 200) { return }
        } catch {
            # Startup/migrations may still be in progress; bounded retries only.
        }
        Start-Sleep -Seconds 1
    }
    throw "Loom did not become ready at $url within 90 seconds."
}

$configDir = Join-Path $base 'config'
$chosenConfig = Join-Path $base 'chosen-config'
$chosenData = Join-Path $base 'chosen-data'
$dbPath = Join-Path $chosenData 'chosen.db'
$configPath = Join-Path $configDir 'loom.yaml'
$sentinel = Join-Path $chosenData 'retention-test.txt'
try {
    foreach ($dir in @($configDir, $chosenConfig, $chosenData)) {
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
    }
    $yamlConfig = $chosenConfig.Replace("'", "''")
    $yamlData = $chosenData.Replace("'", "''")
    $yamlDB = $dbPath.Replace("'", "''")
    # Pre-existing chosen paths must survive both initial install and upgrade.
    @"
config_dir: '$yamlConfig'
data_dir: '$yamlData'
http:
  addr: '127.0.0.1:$port'
storage:
  engine: sqlite
  sqlite:
    path: '$yamlDB'
"@ | Set-Content -LiteralPath $configPath -Encoding UTF8
    'smoke-test-data-must-survive' | Set-Content -LiteralPath $sentinel
    $configHash = (Get-FileHash -LiteralPath $configPath -Algorithm SHA256).Hash

    & (Join-Path $PSScriptRoot 'install.ps1') -BinaryPath $BinaryPath
    Wait-LoomReady
    $service = Get-CimInstance Win32_Service -Filter "Name='Loom'"
    if ($service.StartName -ne 'NT AUTHORITY\LocalService' -or $service.PathName -notmatch '" service --config "') {
        throw 'Service identity or native SCM command line is incorrect.'
    }
    if (-not (Test-Path -LiteralPath $dbPath)) { throw 'Chosen SQLite path was not used.' }
    $appPath = Join-Path $chosenConfig 'loom.json'
    if (-not (Test-Path -LiteralPath $appPath)) { throw 'Chosen config_dir was not used.' }

    $controller = Get-Service Loom
    try {
        $controller.Stop()
        $controller.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(120))
        $controller.Start()
        $controller.WaitForStatus('Running', [TimeSpan]::FromSeconds(30))
    } finally { $controller.Dispose() }
    Wait-LoomReady

    # Rerun with the staging binary to exercise upgrade, not a running image.
    & (Join-Path $PSScriptRoot 'install.ps1') -BinaryPath $BinaryPath
    Wait-LoomReady
    if ((Get-FileHash -LiteralPath $configPath -Algorithm SHA256).Hash -ne $configHash) {
        throw 'Installer overwrote chosen configuration on install/upgrade.'
    }
    & (Join-Path $PSScriptRoot 'uninstall.ps1')
    $deadline = [DateTime]::UtcNow.AddSeconds(30)
    while ((Get-Service -Name Loom -ErrorAction SilentlyContinue) -and [DateTime]::UtcNow -lt $deadline) {
        Start-Sleep -Seconds 1
    }
    if (Get-Service -Name Loom -ErrorAction SilentlyContinue) { throw 'Service was not deleted.' }
    foreach ($path in @($configPath, $appPath, $dbPath, $sentinel, (Join-Path $base 'logs\loom.log'))) {
        if (-not (Test-Path -LiteralPath $path)) { throw "Uninstall removed retained data: $path" }
    }
    if ((Get-Content -LiteralPath $sentinel -Raw).Trim() -ne 'smoke-test-data-must-survive') {
        throw 'Retained data was altered.'
    }
    Write-Host "PASS: SCM start/readiness/stop/start/upgrade/uninstall; retained chosen paths and data (port $port)."
} finally {
    # Always attempt cleanup, even when installation failed halfway through.
    $log = Join-Path $base 'logs\loom.log'
    if (Test-Path -LiteralPath $log) { Get-Content -LiteralPath $log -Tail 60 -ErrorAction Continue }
    $existing = Get-Service -Name Loom -ErrorAction SilentlyContinue
    if ($existing) {
        try {
            if ($existing.Status -notin @('Stopped', 'StopPending')) { $existing.Stop() }
            $existing.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(120))
        } catch {
            Write-Warning "Graceful cleanup failed: $_"
            # Only on this test-owned ephemeral runner, terminate the exact PID.
            $owned = Get-CimInstance Win32_Service -Filter "Name='Loom'"
            if ($owned -and $owned.ProcessId -gt 0) {
                Stop-Process -Id $owned.ProcessId -Force -ErrorAction Continue
            }
        } finally { $existing.Dispose() }
        & "$env:SystemRoot\System32\sc.exe" delete Loom
        if ($LASTEXITCODE -ne 0) { Write-Warning 'Fallback service deletion failed.' }
    }
    $deadline = [DateTime]::UtcNow.AddSeconds(30)
    while ((Get-Service -Name Loom -ErrorAction SilentlyContinue) -and [DateTime]::UtcNow -lt $deadline) {
        Start-Sleep -Seconds 1
    }
    if (Get-Service -Name Loom -ErrorAction SilentlyContinue) {
        # Do not remove storage from underneath a service that is still present.
        Write-Warning 'Service still registered; leaving test directories for diagnosis.'
    } else {
        Remove-Item -LiteralPath $base, $installDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
