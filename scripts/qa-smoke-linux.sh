#!/usr/bin/env bash
# AI Hub desktop smoke test for Ubuntu 22.04/24.04.
# Automates: package install, cloud-mode launch against a stub server,
# bridge probe (offline Codex collection), and the local-mode server chain
# (PostgreSQL role preseeded exactly the way the guided setup instructs users).
# UI-only checks (dialogs, tray, strong reminder) are manual: see
# docs/CROSS_PLATFORM_QA_ZH.md.
# Usage:
#   sudo ./qa-smoke-linux.sh --deb ./AIHub-1.0.0-amd64.deb [--skip-local]
#   ./qa-smoke-linux.sh --appimage ./AIHub-1.0.0-x86_64.AppImage [--skip-local]
set -euo pipefail
PASS=0; FAIL=0
ok(){ echo "PASS: $1"; PASS=$((PASS+1)); }
bad(){ echo "FAIL: $1"; FAIL=$((FAIL+1)); }
summary(){ echo "----"; echo "PASS=$PASS FAIL=$FAIL"; [[ $FAIL -eq 0 ]]; }

DEB=""; APPIMAGE=""; SKIP_LOCAL=0
while [[ $# -gt 0 ]]; do case "$1" in
  --deb) DEB="$2"; shift 2 ;;
  --appimage) APPIMAGE="$2"; shift 2 ;;
  --skip-local) SKIP_LOCAL=1; shift ;;
  *) echo "unknown arg $1" >&2; exit 2 ;;
esac; done
[[ -n "$DEB" || -n "$APPIMAGE" ]] || { echo "need --deb or --appimage" >&2; exit 2; }

need(){ command -v "$1" >/dev/null 2>&1 || { echo "missing tool: $1 (sudo apt install $2)" >&2; exit 2; }; }
need curl curl; need python3 python3
HEADLESS=0
if [[ -z "${DISPLAY:-}" ]]; then
  HEADLESS=1
  need Xvfb xvfb; need xdotool xdotool
fi

LAUNCH_DIR=$(mktemp -d)
PREVIEW="$LAUNCH_DIR/userdata"
mkdir -p "$PREVIEW"
APP_PID=""; HTTP_PID=""; XVPID=""
cleanup(){
  [[ -n "$APP_PID" ]] && kill "$APP_PID" 2>/dev/null || true
  [[ -n "$HTTP_PID" ]] && kill "$HTTP_PID" 2>/dev/null || true
  [[ -n "$XVPID" ]] && kill "$XVPID" 2>/dev/null || true
  pkill -f aihub-server 2>/dev/null || true
}
trap cleanup EXIT

# ---- 1. install / unpack ----------------------------------------------------
DESKTOP_BIN=""
if [[ -n "$DEB" ]]; then
  sudo dpkg -i "$DEB" || sudo apt-get install -f -y
  ok "deb installed"
  DESKTOP_BIN=$(find /usr /opt -name 'aihub-desktop' -o -name 'aihub' -type f 2>/dev/null | grep -iv bridge | head -1)
else
  chmod +x "$APPIMAGE"
  "$APPIMAGE" --appimage-extract >/dev/null
  DESKTOP_BIN="$PWD/squashfs-root/AppRun"
  ok "AppImage extracted"
fi
[[ -n "$DESKTOP_BIN" && -x "$DESKTOP_BIN" ]] && ok "desktop executable present ($DESKTOP_BIN)" || bad "desktop executable not found"
find /opt /usr/lib -path '*resources/bin/aihub-server' 2>/dev/null | head -1 | grep -q . \
  && ok "embedded server binary staged in package" || bad "resources/bin/aihub-server missing from package"

# ---- 2. cloud-mode launch against a local stub server ------------------------
python3 - <<'PY' &
import http.server, json
class H(http.server.SimpleHTTPRequestHandler):
    def do_GET(self):
        body=(json.dumps({'status':'ok'}) if self.path=='/ready' else '<html><title>AI Hub</title></html>').encode()
        self.send_response(200)
        self.send_header('Content-Type','application/json' if self.path=='/ready' else 'text/html')
        self.send_header('Content-Length',str(len(body))); self.end_headers(); self.wfile.write(body)
    def log_message(self,*a): pass
http.server.HTTPServer(('127.0.0.1',18080),H).serve_forever()
PY
HTTP_PID=$!

if [[ $HEADLESS -eq 1 ]]; then
  Xvfb :99 -screen 0 1280x800x24 >/dev/null 2>&1 & XVPID=$!
  export DISPLAY=:99
  sleep 1
fi

cat > "$PREVIEW/app-config.json" <<'JSON'
{"uiMode":"cloud","cloudServer":"http://127.0.0.1:18080","profileId":"smoke","profileName":"smoke"}
JSON
cat > "$PREVIEW/mode-preference.json" <<'JSON'
{"mode":"cloud","remember":true}
JSON
AIHUB_PREVIEW_USER_DATA="$PREVIEW" "$DESKTOP_BIN" >/dev/null 2>&1 &
APP_PID=$!
sleep 10
if kill -0 "$APP_PID" 2>/dev/null; then ok "app process stays alive (cloud mode)"; else bad "app crashed on launch"; fi
if command -v xdotool >/dev/null; then
  xdotool search --name 'AI Hub' >/dev/null 2>&1 && ok "window titled 'AI Hub' visible" || bad "no window found"
fi
kill "$APP_PID" 2>/dev/null || true; wait "$APP_PID" 2>/dev/null || true; APP_PID=""

# ---- 3. bridge probe (Codex collection, offline) ------------------------------
BRIDGE_BIN=$(find /opt /usr/lib "$PWD/squashfs-root" -name 'aihub-bridge' 2>/dev/null | head -1)
if [[ -n "$BRIDGE_BIN" ]]; then
  codex_bin=$(sh -lc 'command -v codex' || true)
  if [[ -n "$codex_bin" ]]; then
    if "$BRIDGE_BIN" --probe codex >/dev/null 2>&1 || "$BRIDGE_BIN" --probe "$codex_bin" >/dev/null 2>&1; then
      ok "bridge --probe codex succeeded"
    else
      bad "bridge probe failed (check codex CLI login state)"
    fi
  else
    echo "SKIP: bridge probe (codex CLI not installed)"
  fi
else
  bad "aihub-bridge not found in package"
fi

# ---- 4. local mode: preseed role/db like the guided setup, then boot ----------
if [[ "$SKIP_LOCAL" -eq 0 && -n "$DEB" ]]; then
  sudo apt-get install -y postgresql >/dev/null
  sudo systemctl enable --now postgresql
  PW=$(python3 -c 'import secrets;print(secrets.token_hex(24))')
  sudo -u postgres psql -v ON_ERROR_STOP=1 -c "DO \$\$ BEGIN CREATE ROLE aihub_m0 LOGIN PASSWORD '$PW'; EXCEPTION WHEN duplicate_object THEN NULL; END \$\$;" >/dev/null
  sudo -u postgres psql -v ON_ERROR_STOP=1 -c "DO \$\$ BEGIN CREATE DATABASE aihub_m0 OWNER aihub_m0; EXCEPTION WHEN duplicate_database THEN NULL; END \$\$;" >/dev/null
  mkdir -p "$PREVIEW/runtime"
  printf 'export AIHUB_DATABASE_URL="postgres://aihub_m0:%s@127.0.0.1:5432/aihub_m0?sslmode=disable"\n' "$PW" > "$PREVIEW/runtime/server.env"
  chmod 600 "$PREVIEW/runtime/server.env"
  cat > "$PREVIEW/app-config.json" <<'JSON'
{"uiMode":"local","cloudServer":"http://127.0.0.1:18080"}
JSON
  cat > "$PREVIEW/mode-preference.json" <<'JSON'
{"mode":"local","remember":true}
JSON
  AIHUB_PREVIEW_USER_DATA="$PREVIEW" "$DESKTOP_BIN" >/dev/null 2>&1 &
  APP_PID=$!
  READY=0
  for _ in $(seq 1 60); do
    if curl -sf http://127.0.0.1:8080/ready 2>/dev/null | grep -q '"ok"'; then READY=1; break; fi
    sleep 1
  done
  [[ $READY -eq 1 ]] && ok "local mode: embedded server reached /ready" || bad "local mode: server never became ready"
  kill "$APP_PID" 2>/dev/null || true; APP_PID=""
else
  echo "SKIP: local-mode check"
fi

summary
