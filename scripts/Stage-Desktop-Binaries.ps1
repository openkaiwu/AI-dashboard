$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$binDir = Join-Path $root 'apps/desktop/resources/bin'
$srcDir = Join-Path $root 'artifacts/aihub-m0'
New-Item -ItemType Directory -Force -Path $binDir | Out-Null
$map = @{
    'aihub-server.exe' = 'aihub-windows-amd64.exe'
    'aihub-bridge.exe' = 'aihub-bridge-windows-amd64.exe'
}
foreach ($dst in $map.Keys) {
    $src = Join-Path $srcDir $map[$dst]
    if (!(Test-Path -LiteralPath $src)) { throw "Missing $src. Build Go server and bridge binaries first." }
    Copy-Item -LiteralPath $src -Destination (Join-Path $binDir $dst) -Force
}
Write-Host "Staged embedded services in $binDir"
