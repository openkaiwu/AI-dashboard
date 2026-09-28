-- Collection status follows the newest received report. Its observed_at can
-- still point to an earlier last-good sample when the provider is unavailable.
ALTER TABLE usage_snapshots ADD COLUMN ingest_order BIGINT GENERATED ALWAYS AS IDENTITY;
CREATE INDEX usage_snapshots_latest_ingest ON usage_snapshots(quota_bucket_id,ingest_order DESC);
