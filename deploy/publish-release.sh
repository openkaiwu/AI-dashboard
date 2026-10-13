#!/usr/bin/env bash
set -euo pipefail
stage="$(cd "$1" && pwd)"
version="$2"
destination="${3:-/var/www/aihub-downloads}"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'Invalid version'; exit 1; }
[[ "$destination" =~ ^/[a-zA-Z0-9/_-]+$ ]] || { echo 'Invalid download directory'; exit 1; }
cd "$stage"
# Validate every manifest path before reading or publishing any package.
files=()
while read -r checksum name; do
  [[ "$checksum" =~ ^[a-f0-9]{64}$ && "$name" != */* && "$name" != *..* ]] || { echo 'Invalid checksum entry'; exit 1; }
  case "$name" in AIHub-*|aihub-*|release.json) ;; *) echo "Unexpected release file: $name"; exit 1 ;; esac
  [[ -f "$name" && ! -L "$name" ]] || { echo "Missing regular file: $name"; exit 1; }
  files+=("$name")
done < SHA256SUMS.txt
[[ ${#files[@]} -gt 0 && -s downloads.html ]] || { echo 'Empty release'; exit 1; }
sha256sum -c SHA256SUMS.txt
for required in "AIHub-正式版v$version.exe" "AIHub-正式版v$version.apk"; do
  printf '%s\n' "${files[@]}" | grep -Fxq "$required" || { echo "Missing required package: $required"; exit 1; }
done
install -d -m 0755 "$destination" "$destination/desktop"
backup="$destination/backups-v$version-$(date -u +%Y%m%dT%H%M%SZ)"
install -d -m 0700 "$backup"
names=("${files[@]}" "index.html" "aihub-mobile.apk" "aihub-mobile.apk.sha256" "SHA256SUMS-v$version.txt" "SHA256SUMS-latest.txt" "desktop/AIHub-Setup-$version.exe" "desktop/AIHub-Setup.exe")
for name in "${names[@]}"; do
  if [[ -f "$destination/$name" ]]; then
    install -d -m 0700 "$backup/$(dirname "$name")"
    cp -p "$destination/$name" "$backup/$name"
  fi
done
publish(){
  install -m 0644 "$1" "$2.pending"
  mv -f "$2.pending" "$2"
}
for name in "${files[@]}"; do publish "$name" "$destination/$name"; done
publish SHA256SUMS.txt "$destination/SHA256SUMS-v$version.txt"
publish SHA256SUMS.txt "$destination/SHA256SUMS-latest.txt"
publish "AIHub-正式版v$version.exe" "$destination/desktop/AIHub-Setup-$version.exe"
publish "AIHub-正式版v$version.exe" "$destination/desktop/AIHub-Setup.exe"
publish "AIHub-正式版v$version.apk" "$destination/aihub-mobile.apk"
(cd "$destination" && sha256sum aihub-mobile.apk > aihub-mobile.apk.sha256.pending)
mv -f "$destination/aihub-mobile.apk.sha256.pending" "$destination/aihub-mobile.apk.sha256"
(cd "$destination" && sha256sum -c "SHA256SUMS-v$version.txt")
# Switch the landing page only once every linked versioned file is in place.
publish downloads.html "$destination/index.html"
echo "RELEASE_PUBLISHED:$version"
