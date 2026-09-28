#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
GO="${GO:-go}"
npm ci --prefix apps/web
npm run build --prefix apps/web
mkdir -p server/webassets/dist artifacts/aihub-m0
cp -R apps/web/dist/. server/webassets/dist/
(cd server && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO" build -trimpath -ldflags="-s -w" -o ../artifacts/aihub-m0/aihub-linux-amd64 ./cmd/aihub)
(cd server && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 "$GO" build -trimpath -ldflags="-s -w" -o ../artifacts/aihub-m0/aihub-windows-amd64.exe ./cmd/aihub)
echo "Server bundles embed Web/PWA; PostgreSQL remains an external requirement."

(cd server && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO" build -trimpath -ldflags="-s -w" -o ../artifacts/aihub-m0/aihub-bridge-linux-amd64 ./cmd/aihub-bridge)
(cd server && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 "$GO" build -trimpath -ldflags="-s -w" -o ../artifacts/aihub-m0/aihub-bridge-windows-amd64.exe ./cmd/aihub-bridge)
