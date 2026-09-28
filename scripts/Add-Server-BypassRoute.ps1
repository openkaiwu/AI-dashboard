#Requires -RunAsAdministrator
param(
    [string]$ServerIP = '203.0.113.10',
    [string]$Gateway = '192.0.2.1'
)

$ErrorActionPreference = 'Stop'
$existing = route print $ServerIP 2>$null | Select-String $ServerIP
if ($existing) {
    Write-Host "Removing old route: $existing"
    route delete $ServerIP | Out-Null
}

Write-Host "Adding bypass route: $ServerIP -> gateway $Gateway (bypass Clash TUN)"
route -p add $ServerIP mask 255.255.255.255 $Gateway metric 1 | Out-Null
Write-Host 'Route added. Verifying...'
route print $ServerIP

Write-Host ''
Write-Host 'Testing connectivity...'
foreach ($port in 22, 80, 443, 8888) {
    $ok = (Test-NetConnection -ComputerName $ServerIP -Port $port -WarningAction SilentlyContinue).TcpTestSucceeded
    Write-Host ("  TCP {0,-5} {1}" -f $port, $(if ($ok) { 'OK' } else { 'FAIL' }))
}

Write-Host ''
Write-Host 'HTTP download test:'
& curl.exe --noproxy '*' -sI --connect-timeout 10 "http://${ServerIP}/downloads/aihub-mobile.apk" 2>$null | Select-Object -First 3
if ($LASTEXITCODE -ne 0) { Write-Host "  HTTP 80 failed (curl exit $LASTEXITCODE)" } else { Write-Host '  HTTP 80 OK' }

Write-Host ''
Write-Host 'SSH test:'
ssh -i (Join-Path $HOME '.ssh/aihub.pem') -o BatchMode=yes -o ConnectTimeout=10 ubuntu@$ServerIP "echo SSH_OK" 2>&1
