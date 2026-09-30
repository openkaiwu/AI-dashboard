#!/usr/bin/env bash
# AI Hub self-host backup (R3/INH-538): plain-SQL dump with --clean so a
# restore REPLACES the target state. Store the output outside the database
# host when possible. Set AIHUB_BACKUP_DIR for the operations page to report
# the latest backup automatically.
set -euo pipefail
: "${AIHUB_DATABASE_URL:?Set AIHUB_DATABASE_URL to the source database}"
DIR="${AIHUB_BACKUP_DIR:-./backups}"
mkdir -p "$DIR"
STAMP=$(date -u +%Y%m%d-%H%M%S)
OUT="$DIR/aihub-backup-$STAMP.sql"
pg_dump --format=plain --no-owner --clean --if-exists "$AIHUB_DATABASE_URL" > "$OUT"
gzip -f "$OUT"
echo "backup written: $OUT.gz"
