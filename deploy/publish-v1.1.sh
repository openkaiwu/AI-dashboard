#!/usr/bin/env bash
set -euo pipefail
stage="$(cd "$1" && pwd)"
destination="${2:-/var/www/aihub-downloads}"
[[ "$destination" =~ ^/[a-zA-Z0-9/_-]+$ ]] || { echo 'Invalid download directory'; exit 1; }
cd "$stage"
sha256sum -c SHA256SUMS-v1.1.txt
install -d -m 0755 "$destination" "$destination/desktop"
backup="$destination/backups-v1.1-$(date -u +%Y%m%dT%H%M%SZ)"
install -d -m 0700 "$backup"
for name in index.html aihub-mobile.apk aihub-mobile.apk.sha256; do
  if [[ -f "$destination/$name" ]]; then cp -p "$destination/$name" "$backup/"; fi
done
publish(){
  install -m 0644 "$1" "$2.pending"
  mv -f "$2.pending" "$2"
}
publish 'AIHub-正式版v1.1.exe' "$destination/AIHub-正式版v1.1.exe"
publish 'AIHub-正式版v1.1.apk' "$destination/AIHub-正式版v1.1.apk"
publish 'SHA256SUMS-v1.1.txt' "$destination/SHA256SUMS-v1.1.txt"
publish 'AIHub-正式版v1.1.exe' "$destination/desktop/AIHub-Setup-1.1.0.exe"
publish 'AIHub-正式版v1.1.apk' "$destination/aihub-mobile.apk"
(cd "$destination" && sha256sum aihub-mobile.apk > aihub-mobile.apk.sha256.pending)
mv -f "$destination/aihub-mobile.apk.sha256.pending" "$destination/aihub-mobile.apk.sha256"
publish 'downloads-v1.1.html' "$destination/index.html"
(cd "$destination" && sha256sum -c SHA256SUMS-v1.1.txt)
echo 'RELEASE_V1_1_PUBLISHED'
