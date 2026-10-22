-- A popup receipt belongs to a device; read/dismiss/snooze belongs to the account.
ALTER TABLE codex_alert_deliveries DROP CONSTRAINT codex_alert_deliveries_pkey;
ALTER TABLE codex_alert_deliveries ADD PRIMARY KEY(user_id,alert_id,device_id);
CREATE TABLE local_data_backups (
 user_id TEXT NOT NULL REFERENCES users(id),
 device_id TEXT NOT NULL REFERENCES devices(id),
 payload JSONB NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(user_id,device_id)
);
CREATE TABLE background_tokens (
 device_id TEXT PRIMARY KEY REFERENCES devices(id),
 token_hash TEXT UNIQUE NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL
);
