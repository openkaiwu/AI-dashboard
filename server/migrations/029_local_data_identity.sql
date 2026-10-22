-- A1 data identity: idempotent reception ledger and coverage metadata for
-- local backups (OPTIMIZATION_EXECUTION_PLAN_2026_10_21_ZH.md section 4).
-- Selection between multiple devices' backups must follow business data time
-- (data_as_of), never the upload timestamp (updated_at).
CREATE TABLE local_data_receptions (
 user_id TEXT NOT NULL REFERENCES users(id),
 operation_id TEXT NOT NULL,
 received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 data_as_of TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(user_id,operation_id)
);
ALTER TABLE local_data_backups
 ADD COLUMN IF NOT EXISTS data_as_of TIMESTAMPTZ,
 ADD COLUMN IF NOT EXISTS schema_version INT NOT NULL DEFAULT 1,
 ADD COLUMN IF NOT EXISTS operation_id TEXT;
CREATE INDEX IF NOT EXISTS local_data_backups_freshness
 ON local_data_backups(user_id, data_as_of DESC);
-- Precompute coverage for legacy rows so the freshness index is usable
-- before every device has re-uploaded with the new protocol.
UPDATE local_data_backups
 SET data_as_of = COALESCE(
   (SELECT max((s->>'observed_at')::timestamptz) FROM jsonb_array_elements(payload->'recent_snapshots') s),
   (payload->>'generated_at')::timestamptz)
 WHERE data_as_of IS NULL;
