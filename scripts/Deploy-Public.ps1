param(
    [string]$ServerIP = '203.0.113.10',
    [string]$SshUser = 'ubuntu',
    [string]$Email = '',
    [string]$Password = '',
    [string]$DeviceName = 'windows-bridge',
    [switch]$SkipUpload,
    [switch]$SkipInstall,
    [switch]$SkipBridge
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$runtimeZip = Join-Path $root 'artifacts/aihub-m0-runtime.zip'
$installScript = Join-Path $root 'deploy/install-ubuntu-ip.sh'
$bridgeConfig = Join-Path $root '.runtime/bridge.json'
$baseUrl = "https://$ServerIP"

function Test-PublicEndpoint {
    param([string]$Url, [switch]$AllowInsecure)
    $args = @('-sS', '-m', '15', '-o', 'NUL', '-w', '%{http_code}')
    if ($AllowInsecure) { $args += '-k' }
    $args += $Url
    $code = & curl.exe @args 2>$null
    return [string]$code
}

function Invoke-HubJson {
    param([string]$Method, [string]$Path, [string]$Token = '', [hashtable]$Body = $null)
    $headers = @{ 'Content-Type' = 'application/json' }
    if ($Token) { $headers['Authorization'] = "Bearer $Token" }
    $params = @{
        Method = $Method
        Uri = "$baseUrl$Path"
        Headers = $headers
        TimeoutSec = 30
    }
    if ($Body) { $params['Body'] = ($Body | ConvertTo-Json -Compress) }
    try {
        return Invoke-RestMethod @params
    } catch {
        if ($_.Exception.Response) {
            $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
            $detail = $reader.ReadToEnd()
            throw "HTTP $($_.Exception.Response.StatusCode.value__) $Path :: $detail"
        }
        throw
    }
}

Write-Host "== AI Hub 公网部署 =="
Write-Host "目标: $baseUrl"

if (!(Test-Path -LiteralPath $runtimeZip)) { throw "缺少运行包: $runtimeZip。先运行 scripts/Build-Release.ps1 或仅构建 Go 二进制后执行 scripts/package.py。" }
if (!(Test-Path -LiteralPath $installScript)) { throw "缺少安装脚本: $installScript" }

Write-Host "`n[1/6] 检查公网端口..."
$http80 = Test-PublicEndpoint "http://$ServerIP/"
$https443 = Test-PublicEndpoint "$baseUrl/ready" -AllowInsecure
Write-Host "  HTTP 80  -> $http80"
Write-Host "  HTTPS 443 -> $https443"
if ($http80 -notmatch '^(200|301|302|404)$' -and !$SkipInstall) {
    Write-Host "  提示: 80 端口尚未就绪。请在腾讯云防火墙放行 TCP 80/443，并确保 SSH 可用后再继续。"
}

if (!$SkipUpload) {
    Write-Host "`n[2/6] 上传到服务器..."
    scp -o BatchMode=yes -o ConnectTimeout=15 $runtimeZip "${SshUser}@${ServerIP}:/home/ubuntu/aihub-m0-runtime.zip"
    scp -o BatchMode=yes -o ConnectTimeout=15 $installScript "${SshUser}@${ServerIP}:/home/ubuntu/install-ubuntu-ip.sh"
    Write-Host "  上传完成"
} else {
    Write-Host "`n[2/6] 跳过上传 (-SkipUpload)"
}

if (!$SkipInstall) {
    Write-Host "`n[3/6] 远程安装 HTTPS..."
    $remote = "sudo bash /home/ubuntu/install-ubuntu-ip.sh $ServerIP /home/ubuntu/aihub-m0-runtime.zip"
    ssh -o BatchMode=yes -o ConnectTimeout=15 "${SshUser}@${ServerIP}" $remote
    Write-Host "  安装脚本执行完成"
} else {
    Write-Host "`n[3/6] 跳过安装 (-SkipInstall)"
}

Write-Host "`n[4/6] 验证公网 HTTPS..."
$readyCode = Test-PublicEndpoint "$baseUrl/ready"
if ($readyCode -ne '200') {
    throw "公网 /ready 未返回 200 (实际: $readyCode)。请检查 Nginx、Certbot 与腾讯云 443 规则。"
}
$ready = Invoke-RestMethod "$baseUrl/ready" -TimeoutSec 30
Write-Host "  /ready = $($ready.status)"

if (!$Email) {
    $Email = Read-Host "云端账户邮箱"
}
if (!$Password) {
    $secure = Read-Host "云端账户密码" -AsSecureString
    $Password = [Runtime.InteropServices.Marshal]::PtrToStringAuto([Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure))
}

Write-Host "`n[5/6] 注册/登录云端账户..."
$token = $null
try {
    $login = Invoke-HubJson POST '/api/v1/auth/login' '' @{
        email = $Email
        password = $Password
        device_name = $DeviceName
    }
    $token = $login.token
    Write-Host "  登录成功"
} catch {
    Write-Host "  登录失败，尝试注册..."
    $register = Invoke-HubJson POST '/api/v1/auth/register' '' @{
        email = $Email
        password = $Password
        device_name = $DeviceName
    }
    $token = $register.token
    Write-Host "  注册并登录成功"
}

if (!$SkipBridge) {
    Write-Host "`n[6/6] 创建 Codex 连接并更新本机采集器..."
    $created = Invoke-HubJson POST '/api/v1/codex/bridges' $token @{ name = $DeviceName }
    if (!$created.token) { throw '创建连接失败：响应缺少 token' }

    $cfg = @{
        server = $baseUrl
        token = [string]$created.token
        interval_seconds = 300
    }
    if (Test-Path -LiteralPath $bridgeConfig) {
        $existing = Get-Content -LiteralPath $bridgeConfig -Raw | ConvertFrom-Json
        if ($existing.codex_path) { $cfg.codex_path = [string]$existing.codex_path }
    }
    $runtimeDir = Split-Path -Parent $bridgeConfig
    if (!(Test-Path -LiteralPath $runtimeDir)) { New-Item -ItemType Directory -Path $runtimeDir | Out-Null }
    [IO.File]::WriteAllText($bridgeConfig, ($cfg | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
    Write-Host "  已写入 $bridgeConfig"

    $stopScript = Join-Path $root 'bridge/Stop-Codex-Bridge.ps1'
    $startScript = Join-Path $root 'bridge/Start-Codex-Bridge.ps1'
    if (Test-Path -LiteralPath $stopScript) { & $stopScript -ConfigPath $bridgeConfig | Out-Null }
    if (Test-Path -LiteralPath $startScript) { & $startScript -ConfigPath $bridgeConfig }

    Write-Host "  等待首次采集上传（最长约 40 秒）..."
    Start-Sleep -Seconds 45
    $bridges = Invoke-HubJson GET '/api/v1/codex/bridges' $token
    if ($bridges.bridges.Count -lt 1) { throw '云端未看到 Codex 连接' }
    Write-Host "  云端连接数: $($bridges.bridges.Count)"
} else {
    Write-Host "`n[6/6] 跳过 Codex 连接 (-SkipBridge)"
}

Write-Host "`n部署完成。"
Write-Host "  公网地址: $baseUrl"
Write-Host "  账户: $Email"
Write-Host ""
Write-Host "手机蜂窝网络验收（需人工完成）："
Write-Host "  1. 关闭 Wi-Fi，用蜂窝数据打开 $baseUrl"
Write-Host "  2. 登录同一账户，检查额度、提醒和便笺"
Write-Host "  3. 切换飞行模式再恢复，确认数据可刷新"
