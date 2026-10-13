#!/usr/bin/env bash
# Update only the application binary; retain the existing service, DB and secrets.
set -euo pipefail
binary="${1:?binary path}"; expected="${2:?sha256}"
[[ "$expected" =~ ^[a-f0-9]{64}$ ]] || exit 2
printf '%s  %s\n' "$expected" "$binary" | sha256sum -c -
target=/opt/aihub/aihub-linux-amd64
backup="/opt/aihub/backups/aihub-$(date -u +%Y%m%dT%H%M%SZ)"
install -d -m 0700 /opt/aihub/backups
cp -p "$target" "$backup"
rollback(){
  echo 'New server failed readiness; restoring previous binary' >&2
  install -m 0755 "$backup" "$target.pending"
  mv -f "$target.pending" "$target"
  systemctl restart aihub
}
install -m 0755 "$binary" "$target.pending"
mv -f "$target.pending" "$target"
if ! systemctl restart aihub; then rollback; exit 1; fi
for _ in $(seq 1 30); do
  if systemctl is-active --quiet aihub && curl -fsS --max-time 2 http://127.0.0.1:8080/ready | python3 -c 'import json,sys; assert json.load(sys.stdin)["status"] == "ok"' 2>/dev/null; then
    sha256sum "$target"
    echo 'SERVER_UPDATED_READY'
    exit 0
  fi
  sleep 1
done
rollback
exit 1
