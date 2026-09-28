param([string]$ServerIP = '203.0.113.10')

$ErrorActionPreference = 'Stop'
$baseUrl = "https://$ServerIP"

function Probe {
    param([string]$Url, [switch]$Insecure)
    $args = @('-sS', '-m', '15', '-w', "URL: %{url_effective}`nHTTP: %{http_code}`n")
    if ($Insecure) { $args += '-k' }
    $args += $Url
    & curl.exe @args
}

Write-Host "== 公网验收检查 =="
Write-Host ""
Write-Host "[HTTP 80]"
Probe "http://$ServerIP/"
Write-Host ""
Write-Host "[HTTPS /ready]"
Probe "$baseUrl/ready"
Write-Host ""
Write-Host "[HTTPS 登录页]"
Probe "$baseUrl/login"
Write-Host ""
Write-Host "[SSH 22]"
ssh -o BatchMode=yes -o ConnectTimeout=10 "${ServerIP}" exit 2>&1
if ($LASTEXITCODE -eq 0) { Write-Host "SSH: ok" } else { Write-Host "SSH: blocked (exit $LASTEXITCODE)" }
