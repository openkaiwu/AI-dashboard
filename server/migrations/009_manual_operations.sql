CREATE TABLE manual_snapshot_operations (
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 operation_id TEXT NOT NULL,
 quota_bucket_id TEXT NOT NULL REFERENCES quota_buckets(id) ON DELETE CASCADE,
 request_hash TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY (user_id, operation_id)
);
