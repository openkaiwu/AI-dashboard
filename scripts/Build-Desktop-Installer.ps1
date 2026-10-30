$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
& (Join-Path $root 'scripts/Stage-Desktop-UI.ps1')
& (Join-Path $root 'scripts/Stage-Desktop-Binaries.ps1')
& (Join-Path $root 'scripts/Stage-TimeZone-Tool.ps1')
Push-Location (Join-Path $root 'apps/desktop')
try {
    if (!(Test-Path 'node_modules/electron-builder')) {
        npm ci
        if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
    }
    node (Join-Path $root 'scripts/run-desktop-render-test.cjs')
    if ($LASTEXITCODE -ne 0) { throw 'Desktop UI did not render in Electron; refusing to package' }
    npm run dist -- --config.electronDist=node_modules/electron/dist
    if ($LASTEXITCODE -ne 0) { throw 'desktop installer build failed' }
    node (Join-Path $root 'scripts/run-desktop-render-test.cjs') (Join-Path $root 'artifacts/desktop/installer/win-unpacked')
    if ($LASTEXITCODE -ne 0) { throw 'Packaged desktop UI did not render in Electron; refusing release metadata' }
    python -X utf8 (Join-Path $root 'scripts/package-desktop-installer.py')
    if ($LASTEXITCODE -ne 0) { throw 'installer metadata failed' }
} finally { Pop-Location }
