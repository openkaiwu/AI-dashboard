param(
    [string]$ServerIP = '203.0.113.10',
    [string]$KeyPath = (Join-Path $HOME '.ssh/aihub.pem'),
    [string]$SshUser = 'ubuntu'
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$remote = "${SshUser}@${ServerIP}"

Write-Host 'Uploading nginx config and download page...'
scp -i $KeyPath -o BatchMode=yes `
    (Join-Path $root 'deploy\nginx-aihub.conf') `
    (Join-Path $root 'deploy\downloads-index.html') `
    (Join-Path $root 'scripts\deploy-server-downloads.sh') `
    "${remote}:/home/ubuntu/"

ssh -i $KeyPath -o BatchMode=yes $remote "sed -i 's/\r$//' /home/ubuntu/deploy-server-downloads.sh && bash /home/ubuntu/deploy-server-downloads.sh"

Write-Host ''
Write-Host 'Download URLs (phone on 4G recommended):'
Write-Host "  http://${ServerIP}/downloads/aihub-mobile.apk"
Write-Host "  http://${ServerIP}:8888/downloads/aihub-mobile.apk"
Write-Host '  (8888 仅下载，8080 为应用内部端口，勿混用)'
Write-Host ''
Write-Host 'IMPORTANT: Open Tencent Cloud security group inbound TCP 80, 443, 8888 (0.0.0.0/0).'
Write-Host 'Run scripts/Verify-Server-Ports.ps1 to test from this PC.'
Write-Host ''
Write-Host 'If browser fails on this PC (proxy/Clash), use:'
Write-Host '  powershell -File scripts/Download-Apk.ps1'
Write-Host '  powershell -File scripts/Download-Apk-Via-Tunnel.ps1'
