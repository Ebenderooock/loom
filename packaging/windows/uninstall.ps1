#Requires -Version 5.1
#Requires -RunAsAdministrator
[CmdletBinding()]
param()
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$service = Get-CimInstance Win32_Service -Filter "Name='Loom'"
if ($null -ne $service) {
    if ($service.Description -ne 'Loom native installer - media automation') {
        throw 'Refusing to remove an unmanaged Loom service.'
    }
    $controller = Get-Service Loom
    try {
        if ($controller.Status -notin @('Stopped', 'StopPending')) { $controller.Stop() }
        $controller.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(120))
    } finally { $controller.Dispose() }
    & "$env:SystemRoot\System32\sc.exe" delete Loom
    if ($LASTEXITCODE -ne 0) { throw "Service deletion failed: $LASTEXITCODE" }
    $installDir = Join-Path $env:ProgramFiles 'Loom'
    foreach ($file in @('loom.exe', 'loom.previous.exe')) {
        $path = Join-Path $installDir $file
        if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path }
    }
}
Write-Host "Preserved $env:ProgramData\Loom (configuration, SQLite, logs) and any media."
Write-Host 'Close Services/Event Viewer if deletion is pending; a reboot may be required before reinstalling.'
