param(
    [string]$Root = (Split-Path -Parent $PSScriptRoot),
    [string]$Installer = '',
    [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'
if (!$Installer) {
    $Installer = Join-Path $Root 'artifacts/desktop/installer/AIHub-Setup-0.3.3.exe'
}
if (!(Test-Path -LiteralPath $Installer)) {
    if ($SkipBuild) { throw "Installer not found: $Installer" }
    & (Join-Path $Root 'scripts/Build-Desktop-Installer.ps1')
    if ($LASTEXITCODE -ne 0) { throw 'Installer build failed' }
}

$runtimeDir = Join-Path $env:APPDATA 'AI Hub\runtime'
New-Item -ItemType Directory -Force -Path $runtimeDir | Out-Null

$devRuntime = Join-Path $Root '.runtime'
foreach ($name in @('bridge.json', 'bootstrap.json', 'server.env', 'app-config.json')) {
    $src = Join-Path $devRuntime $name
    if (Test-Path -LiteralPath $src) {
        Copy-Item -LiteralPath $src -Destination (Join-Path $runtimeDir $name) -Force
    }
}

if (!(Test-Path -LiteralPath (Join-Path $runtimeDir 'bootstrap.json'))) {
    $example = Join-Path $Root 'scripts/bootstrap.example.json'
    if (Test-Path -LiteralPath $example) {
        Copy-Item -LiteralPath $example -Destination (Join-Path $runtimeDir 'bootstrap.json')
        Write-Host 'Created bootstrap.json from example. Edit %APPDATA%\AI Hub\runtime\bootstrap.json with your cloud account.'
    }
}

Get-Process -Name 'AI Hub' -ErrorAction SilentlyContinue | Stop-Process -Force
$installArgs = '/S'
Write-Host "Installing $Installer ..."
$proc = Start-Process -FilePath $Installer -ArgumentList $installArgs -PassThru -Wait
if ($proc.ExitCode -ne 0) { throw "Installer exit code $($proc.ExitCode)" }

$exe = Join-Path $env:LOCALAPPDATA 'Programs\AI Hub\AI Hub.exe'
if (!(Test-Path -LiteralPath $exe)) {
    $exe = Join-Path ${env:ProgramFiles} 'AI Hub\AI Hub.exe'
}
if (!(Test-Path -LiteralPath $exe)) { throw "AI Hub.exe not found after install" }

Write-Host "Launching $exe"
Start-Process -FilePath $exe
Write-Host 'Done. On first launch choose 登录服务器 or 离线本地模式; tray menu can switch later.'
