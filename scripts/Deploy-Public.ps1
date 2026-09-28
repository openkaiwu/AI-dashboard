param(
    [string]$ServerIP = '203.0.113.10',
    [string]$SshUser = 'ubuntu',
    [switch]$SkipUpload,
    [switch]$SkipInstall
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$runtimeZip = Join-Path $root 'artifacts/aihub-m0-runtime.zip'
$installScript = Join-Path $root 'deploy/install-ubuntu-ip.sh'
$baseUrl = "https://$ServerIP"

function Test-PublicEndpoint {
    param([string]$Url)
    $args = @('-sS', '-m', '15', '-o', 'NUL', '-w', '%{http_code}')
    $args += $Url
    $code = & curl.exe @args 2>$null
    return [string]$code
}

Write-Host "== AI Hub 公网部署 =="
Write-Host "目标: $baseUrl"

if (!(Test-Path -LiteralPath $runtimeZip)) { throw "缺少运行包: $runtimeZip。先运行 scripts/Build-Release.ps1 或仅构建 Go 二进制后执行 scripts/package.py。" }
if (!(Test-Path -LiteralPath $installScript)) { throw "缺少安装脚本: $installScript" }

Write-Host "`n[1/6] 检查公网端口..."
$http80 = Test-PublicEndpoint "http://$ServerIP/"
$https443 = Test-PublicEndpoint "$baseUrl/ready"
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

Write-Host "`n[5/6] 服务已就绪。请在服务器通过 aihub-admin 初始化管理员，或由现有管理员授权账户。"
Write-Host "[6/6] 桌面应用登录后自动创建受设备绑定的采集连接。"

Write-Host "`n部署完成。"
Write-Host "  公网地址: $baseUrl"
Write-Host ""
Write-Host "手机蜂窝网络验收（需人工完成）："
Write-Host "  1. 关闭 Wi-Fi，用蜂窝数据打开 $baseUrl"
Write-Host "  2. 登录同一账户，检查额度、提醒和便笺"
Write-Host "  3. 切换飞行模式再恢复，确认数据可刷新"
