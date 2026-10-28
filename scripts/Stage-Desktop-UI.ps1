$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location (Join-Path $root 'apps/web')
try {
    npm run build
    if ($LASTEXITCODE -ne 0) { throw 'Desktop UI build failed' }
} finally { Pop-Location }
$source = Join-Path $root 'apps/web/dist'
$target = [IO.Path]::GetFullPath((Join-Path $root 'apps/desktop/resources/ui'))
$expected = [IO.Path]::GetFullPath((Join-Path $root 'apps/desktop/resources/ui'))
if ($target -ne $expected -or !(Test-Path -LiteralPath (Join-Path $source 'index.html'))) { throw 'Invalid UI staging paths' }
if (Test-Path -LiteralPath $target) { Remove-Item -LiteralPath $target -Recurse -Force }
New-Item -ItemType Directory -Path $target | Out-Null
Get-ChildItem -LiteralPath $source | ForEach-Object { Copy-Item -LiteralPath $_.FullName -Destination $target -Recurse -Force }
Write-Host 'Desktop UI staged independently from cloud web assets'
