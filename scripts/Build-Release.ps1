$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    npm ci --prefix apps/web
    if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
    npm run build --prefix apps/web
    if ($LASTEXITCODE -ne 0) { throw 'Web build failed' }
    Copy-Item apps/web/dist/* server/webassets/dist -Recurse -Force
    New-Item -ItemType Directory -Force artifacts/aihub-m0 | Out-Null
    $linuxRoot = (wsl wslpath -a $root).Trim()
    $command = "cd '$linuxRoot/server' && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 /opt/aihub-tools/go/bin/go build -trimpath -ldflags='-s -w' -o ../artifacts/aihub-m0/aihub-linux-amd64 ./cmd/aihub && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 /opt/aihub-tools/go/bin/go build -trimpath -ldflags='-s -w' -o ../artifacts/aihub-m0/aihub-windows-amd64.exe ./cmd/aihub"
    wsl -e bash -lc $command
    if ($LASTEXITCODE -ne 0) { throw 'Server build failed' }
    $command = "cd '$linuxRoot/server' && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 /opt/aihub-tools/go/bin/go build -trimpath -ldflags='-s -w' -o ../artifacts/aihub-m0/aihub-bridge-linux-amd64 ./cmd/aihub-bridge && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 /opt/aihub-tools/go/bin/go build -trimpath -ldflags='-s -w' -o ../artifacts/aihub-m0/aihub-bridge-windows-amd64.exe ./cmd/aihub-bridge"
    wsl -e bash -lc $command
    if ($LASTEXITCODE -ne 0) { throw 'Bridge build failed' }
    python -X utf8 scripts/package.py
    if ($LASTEXITCODE -ne 0) { throw 'Packaging failed' }
} finally { Pop-Location }
