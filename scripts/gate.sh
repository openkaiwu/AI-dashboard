#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
GO="${GO:-go}"
FLUTTER="${FLUTTER:-flutter}"
: "${AIHUB_TEST_DATABASE_URL:?Set an isolated PostgreSQL test database URL}"
python3 scripts/check-boundaries.py
mkdir -p .runtime
(cd server && "$GO" test -race ./... && "$GO" vet ./... && "$GO" build -o ../.runtime/gate-server ./cmd/aihub)
schema="gate_$(date +%s)_$$"
psql --dbname="$AIHUB_TEST_DATABASE_URL" -v ON_ERROR_STOP=1 -q -c "CREATE SCHEMA $schema"
server_pid=""
cleanup() {
 if [ -n "$server_pid" ]; then kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true; fi
 psql --dbname="$AIHUB_TEST_DATABASE_URL" -v ON_ERROR_STOP=1 -q -c "DROP SCHEMA $schema CASCADE" >/dev/null 2>&1
}
trap cleanup EXIT
case "$AIHUB_TEST_DATABASE_URL" in *\?*) sep='&';; *) sep='?';; esac
AIHUB_DATABASE_URL="$AIHUB_TEST_DATABASE_URL${sep}search_path=$schema" AIHUB_ADDR=127.0.0.1:18081 .runtime/gate-server >.runtime/gate-server.log 2>&1 &
server_pid=$!
for attempt in $(seq 1 30); do
 if curl -fsS http://127.0.0.1:18081/ready >/dev/null 2>&1; then break; fi
 if ! kill -0 "$server_pid" 2>/dev/null; then cat .runtime/gate-server.log; exit 1; fi
 sleep 1
done
curl -fsS http://127.0.0.1:18081/ready >/dev/null
(cd apps/mobile && "$FLUTTER" pub get && "$FLUTTER" analyze && AIHUB_INTEGRATION_URL=http://127.0.0.1:18081 "$FLUTTER" test)
echo "M0 PostgreSQL + Drift + real HTTP gate PASS"
