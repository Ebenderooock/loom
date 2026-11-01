#Requires -Version 5.1
#Requires -RunAsAdministrator
<#
Install/upgrade the native Loom service from an extracted release ZIP.
No download, wrapper, MSI, signature or firewall rule is implied.
#>
[CmdletBinding()]
param(
    [string]$BinaryPath = (Join-Path $PSScriptRoot '..\..\loom.exe')
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$installDir = Join-Path $env:ProgramFiles 'Loom'
$base = Join-Path $env:ProgramData 'Loom'
$configDir = Join-Path $base 'config'
$dataDir = Join-Path $base 'data'
$logDir = Join-Path $base 'logs'
$configPath = Join-Path $configDir 'loom.yaml'
$installedBinary = Join-Path $installDir 'loom.exe'
$description = 'Loom native installer - media automation'

function Invoke-Sc {
    param([string[]]$Arguments)
    & "$env:SystemRoot\System32\sc.exe" @Arguments
    if ($LASTEXITCODE -ne 0) { throw "sc.exe failed with exit code $LASTEXITCODE" }
}

# Use SIDs so installation works on non-English Windows. Protect against
# inherited Users/Everyone write access to the executable and private data.
function Set-LoomDirectoryAcl {
    param([string]$Path, [string]$ServiceRights)
    $acl = New-Object System.Security.AccessControl.DirectorySecurity
    $acl.SetAccessRuleProtection($true, $false)
    foreach ($sid in @('S-1-5-18', 'S-1-5-32-544', 'S-1-5-19')) {
        $rights = if ($sid -eq 'S-1-5-19') { $ServiceRights } else { 'FullControl' }
        $identity = New-Object System.Security.Principal.SecurityIdentifier($sid)
        $rule = New-Object System.Security.AccessControl.FileSystemAccessRule(
            $identity, $rights, 'ContainerInherit,ObjectInherit', 'None', 'Allow'
        )
        $acl.AddAccessRule($rule)
    }
    Set-Acl -LiteralPath $Path -AclObject $acl
}

$BinaryPath = (Resolve-Path -LiteralPath $BinaryPath).ProviderPath
if (-not (Test-Path -LiteralPath $BinaryPath -PathType Leaf)) { throw 'BinaryPath must be a file.' }
if ($BinaryPath -eq $installedBinary) { throw 'Use the binary from a freshly extracted release, not the running executable.' }
$service = Get-CimInstance Win32_Service -Filter "Name='Loom'"
if ($null -ne $service -and $service.Description -ne $description) {
    throw 'An unmanaged Loom service already exists. Migrate it manually before using this installer.'
}
# Check architecture/loader compatibility before interrupting an existing service.
& $BinaryPath version
if ($LASTEXITCODE -ne 0) { throw 'The supplied Loom binary cannot run on this Windows host.' }
foreach ($dir in @($installDir, $base, $configDir, $dataDir, $logDir)) {
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $rights = if ($dir -eq $installDir) { 'ReadAndExecute' } else { 'Modify' }
    Set-LoomDirectoryAcl -Path $dir -ServiceRights $rights
}
if ($null -ne $service) {
    $controller = Get-Service Loom
    try {
        if ($controller.Status -notin @('Stopped', 'StopPending')) { $controller.Stop() }
        $controller.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(120))
    } finally { $controller.Dispose() }
}
if (Test-Path -LiteralPath $installedBinary) {
    Copy-Item -LiteralPath $installedBinary -Destination (Join-Path $installDir 'loom.previous.exe') -Force
}
Copy-Item -LiteralPath $BinaryPath -Destination $installedBinary -Force
if (-not (Test-Path -LiteralPath $configPath)) {
    # YAML single-quoted Windows paths do not interpret backslashes as escapes.
    $yamlConfig = $configDir.Replace("'", "''")
    $yamlData = $dataDir.Replace("'", "''")
    $yamlDB = (Join-Path $dataDir 'loom.db').Replace("'", "''")
    @"
# Native installer defaults; not overwritten on upgrade.
config_dir: '$yamlConfig'
data_dir: '$yamlData'
http:
  addr: '127.0.0.1:1925'
storage:
  engine: sqlite
  sqlite:
    path: '$yamlDB'
log:
  level: info
  format: json
"@ | Set-Content -LiteralPath $configPath -Encoding UTF8
}
$logPath = Join-Path $logDir 'loom.log'
$commandLine = "`"$installedBinary`" service --config `"$configPath`" --log-file `"$logPath`""
if ($null -eq $service) {
    New-Service -Name Loom -DisplayName 'Loom' -BinaryPathName $commandLine `
        -StartupType Automatic -Description $description | Out-Null
}
# CIM avoids the different nested-quote handling in PowerShell 5.1 versus 7
# when passing a quoted executable/config path through sc.exe.
$service = Get-CimInstance Win32_Service -Filter "Name='Loom'"
$change = Invoke-CimMethod -InputObject $service -MethodName Change -Arguments @{
    PathName = $commandLine
    StartName = 'NT AUTHORITY\LocalService'
    StartMode = 'Automatic'
}
if ($change.ReturnValue -ne 0) { throw "Service configuration failed: $($change.ReturnValue)" }
Invoke-Sc -Arguments @('config', 'Loom', 'start=', 'delayed-auto')
Invoke-Sc -Arguments @('failure', 'Loom', 'reset=', '86400', 'actions=', 'restart/10000/restart/30000/restart/60000')
# Treat non-zero orderly exits (e.g. config/migration failures) as failures too.
Invoke-Sc -Arguments @('failureflag', 'Loom', '1')
$controller = Get-Service Loom
try {
    $controller.Start()
    $controller.WaitForStatus('Running', [TimeSpan]::FromSeconds(30))
} finally { $controller.Dispose() }
Write-Host 'Service registered. SCM Running does not imply HTTP readiness.'
Write-Host "Check $logPath and run: & '$installedBinary' healthcheck"
Write-Host 'Open http://127.0.0.1:1925 and complete setup. See docs/windows.md.'
