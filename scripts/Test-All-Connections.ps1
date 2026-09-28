# Full connectivity check: local sync + cloud reachability + mobile hints.
param(
    [string]$LocalBase = 'http://127.0.0.1:8080',
    [string]$CloudBase = 'https://hub.example.com'
)

$ErrorActionPreference = 'Continue'
Write-Host '=== 1. Local server + sync ===' -ForegroundColor Cyan
& (Join-Path $PSScriptRoot 'Test-Local-Sync.ps1') -Base $LocalBase
$localOk = ($LASTEXITCODE -eq 0)

Write-Host ''
Write-Host '=== 2. Mobile-style API ===' -ForegroundColor Cyan
& (Join-Path $PSScriptRoot 'Test-Mobile-Sync.ps1') -Base $LocalBase
$mobileOk = ($LASTEXITCODE -eq 0)

Write-Host ''
Write-Host '=== 3. Cloud server reachability ===' -ForegroundColor Cyan
$cloudOk = $true
foreach ($port in 443, 80, 8888, 22) {
    $ok = (Test-NetConnection -ComputerName ($CloudBase.TrimStart('https://').TrimStart('http://').Split('/')[0]) -Port $port -WarningAction SilentlyContinue).TcpTestSucceeded
    Write-Host ("  TCP {0,-5} {1}" -f $port, $(if ($ok) { 'OK' } else { 'FAIL' }))
    if ($port -eq 443 -and -not $ok) { $cloudOk = $false }
}
try {
    curl.exe -sS --connect-timeout 8 --noproxy '*' -k "$CloudBase/ready" | Out-Null
    if ($LASTEXITCODE -eq 0) {
        Write-Host '[PASS] cloud /ready' -ForegroundColor Green
    } else {
        Write-Host '[FAIL] cloud /ready (TLS or proxy)' -ForegroundColor Yellow
        $cloudOk = $false
    }
} catch {
    Write-Host "[FAIL] cloud /ready - $($_.Exception.Message)" -ForegroundColor Yellow
    $cloudOk = $false
}

Write-Host ''
if ($localOk -and $mobileOk) {
    Write-Host 'Local + mobile API: OK' -ForegroundColor Green
} else {
    Write-Host 'Local/mobile: needs fix' -ForegroundColor Yellow
}
if (-not $cloudOk) {
    Write-Host 'Cloud: unreachable from this PC (Clash TUN or server down).' -ForegroundColor Yellow
    Write-Host '  Fix: run scripts/Add-Server-BypassRoute.ps1 as Administrator, or set 203.0.113.10 to DIRECT in Clash.' -ForegroundColor Yellow
    Write-Host '  Then restore bridge: copy bridge.cloud.json.bak -> bridge.json in %APPDATA%\AI Hub\runtime\' -ForegroundColor Yellow
}
exit $(if ($localOk -and $mobileOk) { 0 } else { 1 })
