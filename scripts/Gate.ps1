$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    npm ci --prefix apps/web
    if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
    npm test --prefix apps/web
    if ($LASTEXITCODE -ne 0) { throw 'Web tests failed' }
    npm run build --prefix apps/web
    if ($LASTEXITCODE -ne 0) { throw 'Web build failed' }
    Copy-Item apps/web/dist/* server/webassets/dist -Recurse -Force
    $linuxRoot = (wsl wslpath -a $root).Trim()
    $command = "cd '$linuxRoot' && . .runtime/server.env && GO=/opt/aihub-tools/go/bin/go FLUTTER=/opt/aihub-tools/flutter/bin/flutter bash scripts/gate.sh"
    wsl -e bash -lc $command
    if ($LASTEXITCODE -ne 0) { throw 'M0 gate failed' }
} finally { Pop-Location }
