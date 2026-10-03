#!/bin/bash
# AI Hub desktop smoke test for macOS (Apple Silicon assumed; adjust arch for
# Intel builds). Automates: dmg mount, app structure, embedded binaries,
# Gatekeeper status, launch, offline bridge probe, and the local-mode server
# chain with Homebrew PostgreSQL. UI-only checks are manual: see
# docs/CROSS_PLATFORM_QA_ZH.md.
# Usage: ./qa-smoke-macos.sh --dmg ./AIHub-1.0.0-arm64.dmg [--skip-local]
set -euo pipefail
PASS=0; FAIL=0
ok(){ echo "PASS: $1"; PASS=$((PASS+1)); }
bad(){ echo "FAIL: $1"; FAIL=$((FAIL+1)); }
summary(){ echo "----"; echo "PASS=$PASS FAIL=$FAIL"; [[ $FAIL -eq 0 ]]; }

DMG=""; SKIP_LOCAL=0
while [[ $# -gt 0 ]]; do case "$1" in
  --dmg) DMG="$2"; shift 2 ;;
  --skip-local) SKIP_LOCAL=1; shift ;;
  *) echo "unknown arg $1" >&2; exit 2 ;;
esac; done
[[ -n "$DMG" ]] || { echo "need --dmg" >&2; exit 2; }
command -v hdiutil >/dev/null || { echo "run on macOS" >&2; exit 2; }

MNT=$(mktemp -d)
APP=""
cleanup(){
  [[ -n "$APP" ]] && osascript -e 'tell application "AI Hub" to quit' >/dev/null 2>&1 || true
  hdiutil detach "$MNT" -quiet >/dev/null 2>&1 || true
  pkill -f aihub-server 2>/dev/null || true
}
trap cleanup EXIT

# ---- 1. mount and inspect ----------------------------------------------------
hdiutil attach "$DMG" -mountpoint "$MNT" -nobrowse -quiet
APP=$(find "$MNT" -maxdepth 2 -name '*.app' | head -1)
[[ -n "$APP" ]] && ok "app bundle found: $(basename "$APP")" || { bad "no .app in dmg"; summary; exit 1; }
[[ -x "$APP/Contents/MacOS"/* ]] && ok "Electron executable present" || bad "Contents/MacOS executable missing"
[[ -x "$APP/Contents/Resources/bin/aihub-server" ]] && ok "embedded aihub-server present +x" || bad "Resources/bin/aihub-server missing"
[[ -x "$APP/Contents/Resources/bin/aihub-bridge" ]] && ok "embedded aihub-bridge present +x" || bad "Resources/bin/aihub-bridge missing"

# ---- 2. gatekeeper status (informational for unsigned builds) ----------------
if spctl -a -t exec "$APP" >/dev/null 2>&1; then
  ok "Gatekeeper accepts the app (signed & notarized)"
else
  echo "WARN: Gatekeeper rejects the app (unsigned). Testers: right-click -> Open, or: xattr -dr com.apple.quarantine \"$APP\""
fi
codesign -dv "$APP" >/dev/null 2>&1 && ok "codesign signature present" || echo "WARN: app is ad-hoc/unsigned"

# ---- 3. launch ----------------------------------------------------------------
AIHUB_PREVIEW_USER_DATA=$(mktemp -d)
local_cfg="$AIHUB_PREVIEW_USER_DATA/app-config.json"
cat > "$local_cfg" <<'JSON'
{"uiMode":"ask"}
JSON
open -W "$APP" --env AIHUB_PREVIEW_USER_DATA="$AIHUB_PREVIEW_USER_DATA" 2>/dev/null || open "$APP"
sleep 10
pgrep -f 'AI Hub.app' >/dev/null && ok "app process stays alive" || bad "app not running after launch"

# ---- 4. bridge probe -----------------------------------------------------------
BRIDGE="$APP/Contents/Resources/bin/aihub-bridge"
codex_bin=$(command -v codex || true)
if [[ -n "$codex_bin" ]]; then
  "$BRIDGE" --probe codex >/dev/null 2>&1 || "$BRIDGE" --probe "$codex_bin" >/dev/null 2>&1 \
    && ok "bridge --probe codex succeeded" || bad "bridge probe failed (check codex CLI login)"
else
  echo "SKIP: bridge probe (codex CLI not installed)"
fi

# ---- 5. local mode with Homebrew PostgreSQL ------------------------------------
if [[ "$SKIP_LOCAL" -eq 0 ]]; then
  BREW=$( [[ -x /opt/homebrew/bin/brew ]] && echo /opt/homebrew/bin/brew || echo /usr/local/bin/brew )
  if [[ ! -x "$BREW" ]]; then
    echo "SKIP: local-mode check (Homebrew not installed)"
  else
    PGV=$("$BREW" --prefix postgresql@16 2>/dev/null || "$BREW" --prefix postgresql@15 2>/dev/null || true)
    if [[ -z "$PGV" ]]; then
      echo "SKIP: local-mode check (brew install postgresql@16 first)"
    else
      "$BREW" services start postgresql@16 >/dev/null 2>&1 || "$BREW" services start postgresql@15 >/dev/null 2>&1 || true
      sleep 5
      PW=$(openssl rand -hex 24)
      "$PGV/bin/psql" -v ON_ERROR_STOP=1 postgres -c "DO \$\$ BEGIN CREATE ROLE aihub_m0 LOGIN PASSWORD '$PW'; EXCEPTION WHEN duplicate_object THEN NULL; END \$\$;" >/dev/null
      "$PGV/bin/psql" -v ON_ERROR_STOP=1 postgres -c "DO \$\$ BEGIN CREATE DATABASE aihub_m0 OWNER aihub_m0; EXCEPTION WHEN duplicate_database THEN NULL; END \$\$;" >/dev/null
      mkdir -p "$AIHUB_PREVIEW_USER_DATA/runtime"
      printf 'export AIHUB_DATABASE_URL="postgres://aihub_m0:%s@127.0.0.1:5432/aihub_m0?sslmode=disable"\n' "$PW" > "$AIHUB_PREVIEW_USER_DATA/runtime/server.env"
      chmod 600 "$AIHUB_PREVIEW_USER_DATA/runtime/server.env"
      cat > "$local_cfg" <<'JSON'
{"uiMode":"local"}
JSON
      osascript -e 'tell application "AI Hub" to quit' >/dev/null 2>&1 || true; sleep 2
      open "$APP"
      READY=0
      for _ in $(seq 1 60); do
        if curl -sf http://127.0.0.1:8080/ready 2>/dev/null | grep -q '"ok"'; then READY=1; break; fi
        sleep 1
      done
      [[ $READY -eq 1 ]] && ok "local mode: embedded server reached /ready" || bad "local mode: server never became ready"
    fi
  fi
else
  echo "SKIP: local-mode check"
fi

summary
