#!/usr/bin/env bash
# AI Hub self-host restore (R3/INH-538): replays a backup produced by
# backup.sh (--clean dumps DROP and re-CREATE every object, so the target
# state is fully replaced). Refuses to run without an explicit ack.
set -euo pipefail
FILE="${1:-}"
TARGET="${2:-${AIHUB_DATABASE_URL:?Set AIHUB_DATABASE_URL (or pass the target as 2nd argument)}}"
[ -f "$FILE" ] || { echo "usage: restore.sh <aihub-backup-*.sql[.gz]> [target_database_url]"; exit 1; }
echo "RESTORE will REPLACE the target database state with $FILE"
read -r -p "Type RESTORE to continue: " ack
[ "$ack" = "RESTORE" ] || { echo "aborted"; exit 1; }
case "$FILE" in
  *.gz) gunzip -c "$FILE" | psql --single-transaction --set ON_ERROR_STOP=1 "$TARGET" ;;
  *)    psql --single-transaction --set ON_ERROR_STOP=1 "$TARGET" -f "$FILE" ;;
esac
echo "restore complete; start the server and verify GET /api/v1/admin/operations"
