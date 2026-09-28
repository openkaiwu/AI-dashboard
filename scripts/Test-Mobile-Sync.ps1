# Simulates Flutter mobile sync against a server (default local).
param(
    [string]$Base = 'http://127.0.0.1:8080',
    [string]$Email = 'demo@aihub.local',
    [string]$Password = 'demo1234'
)

$ErrorActionPreference = 'Stop'
Write-Host "== Mobile-style sync test => $Base ==" -ForegroundColor Cyan

$login = Invoke-RestMethod "$Base/api/v1/auth/login" -Method POST -ContentType 'application/json' `
    -Body (@{ email = $Email; password = $Password; device_name = 'Flutter手机' } | ConvertTo-Json)
$h = @{ Authorization = "Bearer $($login.token)" }

$me = Invoke-RestMethod "$Base/api/v1/me" -Headers $h
Write-Host "[PASS] /me - $($me.email) unread=$($me.unread_count)"

$pull = Invoke-RestMethod "$Base/api/v1/sync/pull?cursor=" -Headers $h
Write-Host "[PASS] sync/pull - events=$($pull.events.Count)"

$dash = Invoke-RestMethod "$Base/api/v1/dashboard" -Headers $h
Write-Host "[PASS] dashboard - accounts=$($dash.accounts.Count)"

$devices = Invoke-RestMethod "$Base/api/v1/devices" -Headers $h
Write-Host "[PASS] devices - count=$($devices.devices.Count) (mobile can list/revoke)"

Write-Host ''
Write-Host 'Mobile app server URL for emulator: http://127.0.0.1:8080' -ForegroundColor Green
Write-Host 'Physical phone needs HTTPS (cloud) or adb reverse to 127.0.0.1:8080' -ForegroundColor Yellow
