#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
GO="${GO:-go}"
npm ci --prefix apps/web
npm run build --prefix apps/web
mkdir -p server/webassets/dist artifacts/aihub-m0
cp -R apps/web/dist/. server/webassets/dist/
for old in server/webassets/dist/assets/*; do
  [[ -f "$old" && ! -e "apps/web/dist/assets/${old##*/}" ]] && rm -- "$old" || true
done
(cd server && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO" build -trimpath -ldflags="-s -w" -o ../artifacts/aihub-m0/aihub-linux-amd64 ./cmd/aihub)
(cd server && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 "$GO" build -trimpath -ldflags="-s -w" -o ../artifacts/aihub-m0/aihub-windows-amd64.exe ./cmd/aihub)
echo "Server bundles embed Web/PWA; PostgreSQL remains an external requirement."

(cd server && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO" build -trimpath -ldflags="-s -w" -o ../artifacts/aihub-m0/aihub-bridge-linux-amd64 ./cmd/aihub-bridge)
(cd server && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 "$GO" build -trimpath -ldflags="-s -w" -o ../artifacts/aihub-m0/aihub-bridge-windows-amd64.exe ./cmd/aihub-bridge)
(cd server && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO" build -trimpath -ldflags="-s -w" -o ../artifacts/aihub-m0/aihub-admin-linux-amd64 ./cmd/aihub-admin)
(cd server && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 "$GO" build -trimpath -ldflags="-s -w" -o ../artifacts/aihub-m0/aihub-admin-windows-amd64.exe ./cmd/aihub-admin)
(cd server && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 "$GO" build -trimpath -ldflags="-s -w" -o ../artifacts/aihub-m0/aihub-darwin-arm64 ./cmd/aihub)
(cd server && CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 "$GO" build -trimpath -ldflags="-s -w" -o ../artifacts/aihub-m0/aihub-darwin-amd64 ./cmd/aihub)
(cd server && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 "$GO" build -trimpath -ldflags="-s -w" -o ../artifacts/aihub-m0/aihub-bridge-darwin-arm64 ./cmd/aihub-bridge)
(cd server && CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 "$GO" build -trimpath -ldflags="-s -w" -o ../artifacts/aihub-m0/aihub-bridge-darwin-amd64 ./cmd/aihub-bridge)
(cd server && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 "$GO" build -trimpath -ldflags="-s -w" -o ../artifacts/aihub-m0/aihub-admin-darwin-arm64 ./cmd/aihub-admin)
(cd server && CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 "$GO" build -trimpath -ldflags="-s -w" -o ../artifacts/aihub-m0/aihub-admin-darwin-amd64 ./cmd/aihub-admin)
