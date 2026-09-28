$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location (Join-Path $root 'apps/desktop')
try {
    if (!(Test-Path 'node_modules/electron')) {
        npm ci
        if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
    }
    npm run package
    if ($LASTEXITCODE -ne 0) { throw 'desktop package failed' }
    python -X utf8 (Join-Path $root 'scripts/package-desktop.py')
    if ($LASTEXITCODE -ne 0) { throw 'desktop zip failed' }
} finally { Pop-Location }
