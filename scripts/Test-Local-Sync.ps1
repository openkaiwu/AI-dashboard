# Local AI Hub connectivity: auth, sync, bridge, devices.
param(
    [string]$Base = 'http://127.0.0.1:8080',
    [string]$Email = 'demo@aihub.local',
    [string]$Password = 'demo1234',
    [string]$BridgeDir = (Join-Path $env:APPDATA 'AI Hub\runtime')
)

$ErrorActionPreference = 'Stop'
$pass = 0; $fail = 0

function Check($name, $ok, $detail) {
    if ($ok) { $script:pass++; Write-Host "[PASS] $name - $detail" -ForegroundColor Green }
    else { $script:fail++; Write-Host "[FAIL] $name - $detail" -ForegroundColor Yellow }
}

Write-Host "== Local AI Hub sync test => $Base ==" -ForegroundColor Cyan

try {
    $ready = Invoke-RestMethod "$Base/ready" -TimeoutSec 5
    Check '/ready' ($ready.status -eq 'ok') $ready.status
} catch { Check '/ready' $false $_.Exception.Message }

try {
    $meta = Invoke-RestMethod "$Base/api/v1/meta" -TimeoutSec 5
    Check '/api/v1/meta' ($meta.protocol -eq 1) "version=$($meta.version)"
} catch { Check '/api/v1/meta' $false $_.Exception.Message }

$loginBody = @{ email = $Email; password = $Password; device_name = 'local-sync-test' } | ConvertTo-Json
try {
    $session = Invoke-RestMethod "$Base/api/v1/auth/login" -Method POST -ContentType 'application/json' -Body $loginBody -TimeoutSec 10
    Check 'login' ($null -ne $session.token) $session.user.email
} catch { Check 'login' $false $_.Exception.Message; exit 1 }

$h = @{ Authorization = "Bearer $($session.token)" }

try {
    $pull = Invoke-RestMethod "$Base/api/v1/sync/pull?cursor=" -Headers $h -TimeoutSec 10
    Check 'sync/pull' ($null -ne $pull.cursor) "events=$($pull.events.Count) has_more=$($pull.has_more)"
} catch { Check 'sync/pull' $false $_.Exception.Message }

$entityId = [guid]::NewGuid().ToString()
$pushBody = @{
    protocol = 1
    operation_id = [guid]::NewGuid().ToString()
    entity = 'note'
    entity_id = $entityId
    base_version = 0
    op = 'put'
    payload = @{ title = 'sync-test'; body = "local test $(Get-Date -Format o)" }
} | ConvertTo-Json -Depth 5
try {
    $push = Invoke-RestMethod "$Base/api/v1/sync/push" -Method POST -Headers $h -ContentType 'application/json' -Body $pushBody -TimeoutSec 10
    Check 'sync/push' ($push.status -eq 'applied' -or $push.status -eq 'conflict') $push.status
} catch { Check 'sync/push' $false $_.Exception.Message }

try {
    $pull2 = Invoke-RestMethod "$Base/api/v1/sync/pull?cursor=" -Headers $h -TimeoutSec 10
    $found = @($pull2.events | Where-Object { $_.entity_id -eq $entityId }).Count -gt 0
    Check 'sync/roundtrip' $found "entity=$entityId"
} catch { Check 'sync/roundtrip' $false $_.Exception.Message }

try {
    $devices = Invoke-RestMethod "$Base/api/v1/devices" -Headers $h -TimeoutSec 10
    Check 'devices' ($devices.devices.Count -ge 1) "count=$($devices.devices.Count)"
} catch { Check 'devices' $false $_.Exception.Message }

try {
    $overview = Invoke-RestMethod "$Base/api/v1/codex/overview" -Headers $h -TimeoutSec 10
    Check 'codex/overview' ($null -ne $overview.generated_at) "alerts=$($overview.alerts.Count)"
} catch { Check 'codex/overview' $false $_.Exception.Message }

try {
    $bridge = Invoke-RestMethod "$Base/api/v1/codex/bridges" -Method POST -Headers $h -ContentType 'application/json' -Body '{"name":"local-test-bridge"}' -TimeoutSec 10
    Check 'bridge/create' ($null -ne $bridge.token) $bridge.id
    if ($bridge.token) {
        New-Item -ItemType Directory -Force -Path $BridgeDir | Out-Null
        $codexPath = ''
        $existing = Join-Path $BridgeDir 'bridge.json'
        if (Test-Path $existing) {
            $old = Get-Content $existing -Raw | ConvertFrom-Json
            if ($old.codex_path) { $codexPath = $old.codex_path }
        }
        $cfg = @{
            server = $Base
            token = $bridge.token
            codex_path = $codexPath
            cursor_enabled = $true
            interval_seconds = 300
        }
        $cfg | ConvertTo-Json | Set-Content (Join-Path $BridgeDir 'bridge.json') -Encoding UTF8
        Check 'bridge/config' $true (Join-Path $BridgeDir 'bridge.json')
    }
} catch { Check 'bridge/create' $false $_.Exception.Message }

Write-Host ''
Write-Host "Summary: $pass passed, $fail failed" -ForegroundColor $(if ($fail -eq 0) { 'Green' } else { 'Yellow' })
if ($fail -gt 0) { exit 1 }
