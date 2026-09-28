# Read Cursor quota from local state.vscdb + api2.cursor.sh. Tokens never leave this machine except to Cursor.
$ErrorActionPreference = 'Stop'
$db = Join-Path $env:APPDATA 'Cursor\User\globalStorage\state.vscdb'
if (!(Test-Path -LiteralPath $db)) { throw "Cursor state.vscdb not found: $db" }

function Get-StateValue([string]$Key) {
  return (sqlite3 $db "SELECT value FROM ItemTable WHERE key='$Key';")
}

$token = Get-StateValue 'cursorAuth/accessToken'
$plan = Get-StateValue 'cursorAuth/stripeMembershipType'
if (!$token) { throw 'Cursor is not signed in on this machine.' }

$headers = @{
  Authorization = "Bearer $token"
  'Content-Type' = 'application/json'
  'Connect-Protocol-Version' = '1'
}

try {
  $raw = Invoke-RestMethod -Uri 'https://api2.cursor.sh/aiserver.v1.DashboardService/GetCurrentPeriodUsage' -Method POST -Headers $headers -Body '{}' -TimeoutSec 20
} catch {
  throw "Cursor usage API failed: $($_.Exception.Message)"
}

$usage = $raw.planUsage
$resetMs = [int64]$raw.billingCycleEnd
$resetAt = [DateTimeOffset]::FromUnixTimeMilliseconds($resetMs).UtcDateTime.ToString('yyyy-MM-ddTHH:mm:ssZ')

$snapshot = [ordered]@{
  observed_at = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
  source = 'cursor_api2'
  status = 'ok'
  plan_name = $plan
  limit_usd = if ($usage.limit) { [math]::Round($usage.limit / 100, 2) } else { $null }
  used_percent = $usage.totalPercentUsed
  reset_at = $resetAt
  buckets = @(
    @{ scope_key = 'cursor_models'; label = 'Cursor Models'; used_percent = $usage.autoPercentUsed }
    @{ scope_key = 'other_models'; label = 'Other Models'; used_percent = $usage.apiPercentUsed }
  )
}

Write-Host 'Cursor quota probe (sanitized, no tokens):'
$snapshot | ConvertTo-Json -Depth 5
