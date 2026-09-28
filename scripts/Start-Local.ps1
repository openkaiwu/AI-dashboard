$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $root '.runtime/server.env'
if (!(Test-Path -LiteralPath $envFile)) { throw '先配置 AIHUB_DATABASE_URL，或按 docs/DEPLOYMENT_ZH.md 初始化本地 PostgreSQL。' }
$line = Get-Content -LiteralPath $envFile | Where-Object { $_ -match '^export AIHUB_DATABASE_URL=' } | Select-Object -First 1
if (!$line) { throw 'Missing database configuration' }
$env:AIHUB_DATABASE_URL = $line.Substring('export AIHUB_DATABASE_URL='.Length).Trim('"')
$env:AIHUB_ADDR = '127.0.0.1:8080'
$env:AIHUB_DEMO = 'false'
$binary = Join-Path $root 'artifacts/aihub-m0/aihub-windows-amd64.exe'
if (!(Test-Path -LiteralPath $binary)) { throw '请先构建运行包。' }
# Keep WSL alive while the native Windows server uses its local PostgreSQL.
$keepFile = Join-Path $root '.runtime/wsl-keepalive.json'
$keepAlive = $null
if (Test-Path -LiteralPath $keepFile) {
    try {
        $saved = Get-Content -LiteralPath $keepFile -Raw | ConvertFrom-Json
        $candidate = Get-Process -Id $saved.Id -ErrorAction Stop
        if ($candidate.ProcessName -eq 'wsl' -and $candidate.StartTime.ToUniversalTime().Ticks.ToString() -eq $saved.StartTicks) { $keepAlive = $candidate }
    } catch {}
}
if (!$keepAlive) {
    $keepAlive = Start-Process -FilePath 'wsl.exe' -ArgumentList @('-e','sleep','infinity') -WindowStyle Hidden -PassThru
    @{Id=$keepAlive.Id;StartTicks=$keepAlive.StartTime.ToUniversalTime().Ticks.ToString()} | ConvertTo-Json | Set-Content -LiteralPath $keepFile
}
wsl -e sh -lc 'pg_ctlcluster 14 main status >/dev/null 2>&1 || pg_ctlcluster 14 main start'
if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL startup failed' }
try {
    $health = Invoke-RestMethod 'http://127.0.0.1:8080/api/v1/meta' -TimeoutSec 2
    $ready = Invoke-RestMethod 'http://127.0.0.1:8080/ready' -TimeoutSec 2
    if ($health.protocol -eq 1 -and $ready.status -eq 'ok') { Write-Host 'AI Hub 已在 http://127.0.0.1:8080 运行'; exit 0 }
} catch {}
$proc = Start-Process -FilePath $binary -WorkingDirectory $root -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $root '.runtime/server.log') -RedirectStandardError (Join-Path $root '.runtime/server-error.log')
Set-Content -LiteralPath (Join-Path $root '.runtime/server.pid') -Value $proc.Id
for ($i=0; $i -lt 30; $i++) {
    if ($proc.HasExited) { throw 'AI Hub 启动失败，请查看 .runtime/server-error.log' }
    try { $ready = Invoke-RestMethod 'http://127.0.0.1:8080/ready' -TimeoutSec 1; if ($ready.status -eq 'ok') { Write-Host 'AI Hub 已启动：http://127.0.0.1:8080'; exit 0 } } catch {}
    Start-Sleep -Milliseconds 500
}
throw 'AI Hub readiness timeout'
