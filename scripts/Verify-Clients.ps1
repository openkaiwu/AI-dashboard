param(
    [string]$Server = 'https://hub.example.com',
    [string]$BootstrapPath = (Join-Path $env:APPDATA 'AI Hub\runtime\bootstrap.json'),
    [string]$DesktopExe = (Join-Path $env:LOCALAPPDATA 'Programs\AI Hub\AI Hub.exe'),
    [string]$KeyPath = (Join-Path $HOME '.ssh/aihub.pem')
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$results = @()

function Add-Result($name, $ok, $detail) {
    $script:results += [pscustomobject]@{ Name = $name; Ok = [bool]$ok; Detail = $detail }
    $mark = if ($ok) { 'PASS' } else { 'FAIL' }
    Write-Host "[$mark] $name - $detail"
}

Write-Host '== AI Hub full client verification ==' -ForegroundColor Cyan

# Cloud checks on server (authoritative)
$bootstrap = $null
if (Test-Path -LiteralPath $BootstrapPath) {
    $bootstrap = Get-Content -LiteralPath $BootstrapPath -Raw | ConvertFrom-Json
} elseif (Test-Path -LiteralPath (Join-Path $root '.runtime\bootstrap.json')) {
    $bootstrap = Get-Content -LiteralPath (Join-Path $root '.runtime\bootstrap.json') -Raw | ConvertFrom-Json
}
if (!$bootstrap) {
    Add-Result 'Cloud remote verify' $false 'bootstrap.json missing'
} else {
    $secrets = @{
        server = 'https://127.0.0.1'
        email = $bootstrap.email
        password = $bootstrap.password
    } | ConvertTo-Json -Compress
    $tmp = Join-Path $env:TEMP 'verify-secrets.json'
    Set-Content -LiteralPath $tmp -Value $secrets -Encoding Ascii -NoNewline
    $hostIp = $Server.TrimStart('https://').Split('/')[0]
    scp -i $KeyPath -o BatchMode=yes $tmp "ubuntu@${hostIp}:/home/ubuntu/verify-secrets.json" | Out-Null
    scp -i $KeyPath -o BatchMode=yes (Join-Path $root 'scripts\verify-cloud-remote.sh') "ubuntu@$($Server.TrimStart('https://').Split('/')[0]):/home/ubuntu/" | Out-Null
    $remote = ssh -i $KeyPath -o BatchMode=yes "ubuntu@$($Server.TrimStart('https://').Split('/')[0])" "sed -i 's/\r$//' /home/ubuntu/verify-cloud-remote.sh && bash /home/ubuntu/verify-cloud-remote.sh /home/ubuntu/verify-secrets.json" 2>&1
    foreach ($line in ($remote -split "`n")) {
        if ($line -match '^\[(PASS|FAIL)\] (.+) - (.+)$') {
            Add-Result $Matches[2] ($Matches[1] -eq 'PASS') $Matches[3]
        }
    }
}

# APK hash
$apk = Join-Path $root 'artifacts\android-native\aihub-mobile.apk'
if (Test-Path -LiteralPath $apk) {
    $hash = (Get-FileHash -LiteralPath $apk -Algorithm SHA256).Hash.ToLower()
    $remoteHash = (ssh -i $KeyPath -o BatchMode=yes "ubuntu@$($Server.TrimStart('https://').Split('/')[0])" "sha256sum /var/www/aihub-downloads/aihub-mobile.apk | awk '{print `$1}'").Trim().ToLower()
    Add-Result 'APK SHA256 match' ($hash -eq $remoteHash) "$hash"
}

# Desktop
$desktopInstalled = Test-Path -LiteralPath $DesktopExe
Add-Result 'Desktop installed' $desktopInstalled $DesktopExe
if (!$desktopInstalled) {
    Add-Result 'Desktop running' $false 'not installed'
    Add-Result 'Bridge running' $false 'not installed'
} else {
    $hubProc = Get-Process -Name 'AI Hub' -ErrorAction SilentlyContinue
    $bridgeProc = @(Get-Process -Name 'aihub-bridge','aihub-bridge-windows-amd64' -ErrorAction SilentlyContinue)
    if (!$hubProc) {
        Start-Process -FilePath $DesktopExe | Out-Null
        Start-Sleep -Seconds 12
        $hubProc = Get-Process -Name 'AI Hub' -ErrorAction SilentlyContinue
        $bridgeProc = @(Get-Process -Name 'aihub-bridge','aihub-bridge-windows-amd64' -ErrorAction SilentlyContinue)
    }
    Add-Result 'Desktop running' ($null -ne $hubProc) ("count=$($hubProc.Count)")
    Add-Result 'Bridge running' ($bridgeProc.Count -gt 0) ("count=$($bridgeProc.Count)")
}

# Local server
try {
    $lr = Invoke-RestMethod 'http://127.0.0.1:8080/ready' -TimeoutSec 3
    Add-Result 'Local /ready' ($lr.status -eq 'ok') $lr.status
    $localLogin = Invoke-RestMethod 'http://127.0.0.1:8080/api/v1/auth/login' -Method POST -ContentType 'application/json' -Body '{"email":"demo@aihub.local","password":"demo1234","device_name":"verify-local"}' -TimeoutSec 5
    Add-Result 'Local demo login' ($null -ne $localLogin.token) $localLogin.user.email
    $overview = Invoke-RestMethod 'http://127.0.0.1:8080/api/v1/codex/overview' -Headers @{ Authorization = "Bearer $($localLogin.token)" } -TimeoutSec 5
    Add-Result 'Local codex/overview' ($null -ne $overview.generated_at) ("alerts=$($overview.alerts.Count)")
} catch {
    Add-Result 'Local /ready' $false $_.Exception.Message
}

node --test (Join-Path $root 'apps\desktop\main.test.cjs') | Out-Null
Add-Result 'Desktop IPC unit test' ($LASTEXITCODE -eq 0) "exit=$LASTEXITCODE"

$failed = @($results | Where-Object { -not $_.Ok })
Write-Host ''
Write-Host ("Summary: {0}/{1} passed" -f ($results.Count - $failed.Count), $results.Count) -ForegroundColor $(if ($failed.Count -eq 0) { 'Green' } else { 'Yellow' })
if ($failed.Count -gt 0) {
    $failed | ForEach-Object { Write-Host " - $($_.Name): $($_.Detail)" -ForegroundColor Yellow }
    exit 1
}
exit 0
