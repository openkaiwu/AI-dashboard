CREATE TABLE codex_alert_deliveries (
 user_id TEXT NOT NULL,
 alert_id TEXT NOT NULL,
 token TIMESTAMPTZ NOT NULL,
 claim_id TEXT NOT NULL,
 device_id TEXT NOT NULL,
 priority INTEGER NOT NULL,
 claimed_at TIMESTAMPTZ NOT NULL,
 lease_until TIMESTAMPTZ NOT NULL,
 delivered_at TIMESTAMPTZ,
 PRIMARY KEY(user_id,alert_id),
 FOREIGN KEY(user_id,alert_id) REFERENCES codex_alerts(user_id,id) ON DELETE CASCADE
);
CREATE INDEX codex_delivery_recent ON codex_alert_deliveries(user_id,claimed_at DESC);
