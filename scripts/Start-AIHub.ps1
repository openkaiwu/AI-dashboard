$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
& powershell.exe -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'Start-Local.ps1')
if ($LASTEXITCODE -ne 0) { throw '本地服务启动失败' }
$bridgeConfig = Join-Path $root '.runtime/bridge.json'
if (Test-Path -LiteralPath $bridgeConfig) {
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File (Join-Path $root 'bridge/Start-Codex-Bridge.ps1') -ConfigPath $bridgeConfig
    if ($LASTEXITCODE -ne 0) { Write-Warning '采集器启动失败，请检查电脑连接。' }
}
$binary = Join-Path $root 'artifacts/desktop/AIHub-win32-x64/AIHub.exe'
if (!(Test-Path -LiteralPath $binary)) { throw '请先构建桌面应用，或解压桌面应用包至 artifacts/desktop。' }
Start-Process -FilePath $binary -WorkingDirectory (Split-Path -Parent $binary) -WindowStyle Normal
