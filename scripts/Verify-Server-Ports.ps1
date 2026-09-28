param(
    [string]$ServerIP = '203.0.113.10'
)

$ErrorActionPreference = 'Continue'
Write-Host "Testing TCP to $ServerIP ..."
foreach ($port in 22, 80, 443, 8888) {
    $ok = (Test-NetConnection -ComputerName $ServerIP -Port $port -WarningAction SilentlyContinue).TcpTestSucceeded
    Write-Host ("  TCP {0,-5} {1}" -f $port, $(if ($ok) { 'OK' } else { 'FAIL' }))
}

Write-Host ''
Write-Host 'HTTP download test (no proxy):'
foreach ($url in @(
    "http://${ServerIP}/downloads/aihub-mobile.apk",
    "http://${ServerIP}:8888/downloads/aihub-mobile.apk"
)) {
    & curl.exe --noproxy '*' -sI --connect-timeout 8 $url 2>$null | Select-Object -First 1
    if ($LASTEXITCODE -ne 0) { Write-Host "  FAIL $url (curl exit $LASTEXITCODE)" }
}

Write-Host ''
Write-Host 'If TCP 22 OK but 80/443/8888 FAIL from your PC:'
Write-Host '  1. 腾讯云控制台 -> 云服务器 -> 安全组 -> 入站规则'
Write-Host '     放行 TCP 80、443、8888，来源 0.0.0.0/0'
Write-Host '  2. 若仍失败，关闭 Clash TUN 或将 203.0.113.10 设为 DIRECT'
Write-Host ''
Write-Host 'APK 备用下载（不依赖 80 端口）：'
Write-Host '  powershell -File scripts/Download-Apk.ps1'
