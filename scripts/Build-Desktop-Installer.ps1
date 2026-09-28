$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
& (Join-Path $root 'scripts/Stage-Desktop-Binaries.ps1')
Push-Location (Join-Path $root 'apps/desktop')
try {
    if (!(Test-Path 'node_modules/electron-builder')) {
        npm ci
        if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
    }
    npm run dist
    if ($LASTEXITCODE -ne 0) { throw 'desktop installer build failed' }
    python -X utf8 (Join-Path $root 'scripts/package-desktop-installer.py')
    if ($LASTEXITCODE -ne 0) { throw 'installer metadata failed' }
} finally { Pop-Location }
