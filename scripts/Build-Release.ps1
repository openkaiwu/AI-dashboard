$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    npm ci --prefix apps/web
    if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
    npm run build --prefix apps/web
    if ($LASTEXITCODE -ne 0) { throw 'Web build failed' }
    Copy-Item apps/web/dist/* server/webassets/dist -Recurse -Force
    $assetDir = Join-Path $root 'server/webassets/dist/assets'
    $currentAssets = @(Get-ChildItem (Join-Path $root 'apps/web/dist/assets') -File | ForEach-Object Name)
    Get-ChildItem -LiteralPath $assetDir -File | Where-Object { $_.Name -notin $currentAssets } | ForEach-Object { Remove-Item -LiteralPath $_.FullName }
    New-Item -ItemType Directory -Force artifacts/aihub-m0 | Out-Null
    $linuxRoot = '/mnt/' + $root.Substring(0,1).ToLowerInvariant() + '/' + ($root.Substring(3) -replace '\\','/')
    $command = "cd '$linuxRoot/server' && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 /opt/aihub-tools/go/bin/go build -trimpath -ldflags='-s -w' -o ../artifacts/aihub-m0/aihub-linux-amd64 ./cmd/aihub && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 /opt/aihub-tools/go/bin/go build -trimpath -ldflags='-s -w' -o ../artifacts/aihub-m0/aihub-windows-amd64.exe ./cmd/aihub"
    wsl -e bash -lc $command
    if ($LASTEXITCODE -ne 0) { throw 'Server build failed' }
    $command = "cd '$linuxRoot/server' && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 /opt/aihub-tools/go/bin/go build -trimpath -ldflags='-s -w' -o ../artifacts/aihub-m0/aihub-bridge-linux-amd64 ./cmd/aihub-bridge && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 /opt/aihub-tools/go/bin/go build -trimpath -ldflags='-s -w' -o ../artifacts/aihub-m0/aihub-bridge-windows-amd64.exe ./cmd/aihub-bridge"
    wsl -e bash -lc $command
    if ($LASTEXITCODE -ne 0) { throw 'Bridge build failed' }
    $command = "cd '$linuxRoot/server' && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 /opt/aihub-tools/go/bin/go build -trimpath -ldflags='-s -w' -o ../artifacts/aihub-m0/aihub-admin-linux-amd64 ./cmd/aihub-admin && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 /opt/aihub-tools/go/bin/go build -trimpath -ldflags='-s -w' -o ../artifacts/aihub-m0/aihub-admin-windows-amd64.exe ./cmd/aihub-admin"
    wsl -e bash -lc $command
    if ($LASTEXITCODE -ne 0) { throw 'Admin build failed' }
    python -X utf8 scripts/package.py
    if ($LASTEXITCODE -ne 0) { throw 'Packaging failed' }
} finally { Pop-Location }
