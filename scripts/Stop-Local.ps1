$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$pidFile = Join-Path $root '.runtime/server.pid'
if (Test-Path -LiteralPath $pidFile) {
    $serverPid = [int](Get-Content -LiteralPath $pidFile)
    $process = Get-Process -Id $serverPid -ErrorAction SilentlyContinue
    $expected = Join-Path $root 'artifacts/aihub-m0/aihub-windows-amd64.exe'
    if ($process -and $process.Path -eq $expected) { Stop-Process -Id $serverPid }
}
$keepFile = Join-Path $root '.runtime/wsl-keepalive.json'
if (Test-Path -LiteralPath $keepFile) {
    $saved = Get-Content -LiteralPath $keepFile -Raw | ConvertFrom-Json
    $process = Get-Process -Id $saved.Id -ErrorAction SilentlyContinue
    if ($process -and $process.ProcessName -eq 'wsl' -and $process.StartTime.ToUniversalTime().Ticks.ToString() -eq $saved.StartTicks) { Stop-Process -Id $process.Id }
}
Write-Host 'AI Hub stopped. Stored data is retained.'
