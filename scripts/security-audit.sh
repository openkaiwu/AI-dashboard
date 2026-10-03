#!/usr/bin/env bash
# AI Hub release security audit (R3/INH-544 automated half). Run from the repo
# root. Every check prints PASS/FAIL; a FAIL exits non-zero. The manual review
# items live in docs/OPERATIONS_ZH.md (release checklist section).
set -uo pipefail
FAIL=0
check() { # check <name> <command...>
  local name="$1"; shift
  if "$@" >/dev/null 2>&1; then
    echo "PASS  $name"
  else
    echo "FAIL  $name"
    FAIL=1
  fi
}
grep_fail() { # grep_fail <name> <pattern> <files...> — pattern MUST NOT appear
  local name="$1"; local pattern="$2"; shift 2
  if grep -rInE "$pattern" "$@" >/dev/null 2>&1; then
    echo "FAIL  $name (pattern found)"
    FAIL=1
  else
    echo "PASS  $name"
  fi
}
grep_must() { # grep_must <name> <pattern> <files...> — pattern MUST appear
  local name="$1"; local pattern="$2"; shift 2
  if grep -rInE "$pattern" "$@" >/dev/null 2>&1; then
    echo "PASS  $name"
  else
    echo "FAIL  $name (pattern missing)"
    FAIL=1
  fi
}

echo "== AI Hub security audit =="

EXCLUDES=(--exclude-dir=node_modules --exclude-dir=testdata --exclude-dir=dist --exclude-dir=build --exclude-dir=upstream --exclude='*.min.js')

# 1. Build hygiene
if command -v go >/dev/null 2>&1; then
  check "go vet" bash -c "cd server && go vet ./..."
else
  echo "SKIP  go vet (go not on PATH; run inside the build environment)"
fi
grep_fail "no hardcoded API keys" "sk-(ant-)?[A-Za-z0-9]{20,}|ghp_[A-Za-z0-9]{30,}|AKIA[0-9A-Z]{16}" server apps extension "${EXCLUDES[@]}" --include='*.go' --include='*.ts' --include='*.tsx' --include='*.dart' --include='*.js' --include='*.json' --include='*.sh'
grep_fail "no committed private keys" "BEGIN (RSA |EC )?PRIVATE KEY" server apps extension bridge docs "${EXCLUDES[@]}" -r
grep_fail "no password literals in Go sources" "(password|passwd)\s*=\s*\"[^\"]{6,}\"" server --include='*.go'

# 2. Auth surface
grep_must "invite registration creates pending accounts" "member','pending'" server/internal/auth/invite.go
grep_must "pending accounts blocked at login" "account_pending" server/internal/auth/auth.go
grep_must "bridge uploads verify active desktop device" "EnsureActiveDesktopDevice" server/internal/connector
grep_must "access tokens stored hashed" "access_hash" server/internal/auth/auth.go
grep_fail "no plaintext token columns" "access_token TEXT|token TEXT NOT NULL" server/migrations --include='*.sql'

# 3. Secrets never leave the canonical model (M4 contract)
grep_must "secret_ref mechanism present" "secret_ref" server/internal/config/model.go
grep_fail "no cookie transport in extension" "document\.cookie|cookies\(" extension

# 4. Transport / origin
grep_must "bridge refuses non-HTTPS remote servers" "https" server/cmd/aihub-bridge/main.go
grep_must "CORS allowlist exists" "AIHUB_ALLOWED_ORIGINS" server/internal/httpx/httpx.go

# 5. Data minimisation: raw import snapshots are not part of sync tables
grep_fail "conversation raw snapshots never enter sync" "conversation_raw_snapshots" server/internal/sync server/internal/api --include='*.go'

echo "== result: $([ "$FAIL" -eq 0 ] && echo PASS || echo FAIL) =="
exit "$FAIL"
