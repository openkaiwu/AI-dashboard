CREATE TABLE mobile_reminder_settings (
 user_id TEXT PRIMARY KEY REFERENCES users(id),
 device_id TEXT NOT NULL REFERENCES devices(id),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE mobile_reminder_receipts (
 user_id TEXT NOT NULL REFERENCES users(id),
 event_id TEXT NOT NULL,
 batch_id TEXT NOT NULL,
 device_id TEXT NOT NULL REFERENCES devices(id),
 reserved_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 submitted_at TIMESTAMPTZ,
 PRIMARY KEY(user_id,event_id)
);
CREATE TABLE mobile_reminder_cycles (
 user_id TEXT NOT NULL REFERENCES users(id),
 window_id TEXT NOT NULL,
 cycle_id TEXT NOT NULL,
 reset_at BIGINT,
 observed_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(user_id,window_id)
);
-- Preserve all known mobile submissions; desktop delivery never consumes phone delivery.
CREATE TABLE mobile_legacy_presented (
 user_id TEXT NOT NULL REFERENCES users(id),
 advice_key TEXT NOT NULL,
 PRIMARY KEY(user_id,advice_key)
);
INSERT INTO mobile_legacy_presented(user_id,advice_key)
 SELECT DISTINCT x.user_id,a.payload->>'key'
 FROM codex_alert_deliveries x JOIN devices d ON d.id=x.device_id
 JOIN codex_alerts a ON a.user_id=x.user_id AND a.id=x.alert_id
 WHERE d.kind='mobile' AND x.delivered_at IS NOT NULL
 ON CONFLICT DO NOTHING;

INSERT INTO mobile_reminder_settings(user_id,device_id)
 SELECT DISTINCT ON(d.user_id) d.user_id,d.id FROM background_tokens b JOIN devices d ON d.id=b.device_id
 WHERE b.expires_at>now() AND d.revoked_at IS NULL ORDER BY d.user_id,b.expires_at DESC
 ON CONFLICT DO NOTHING;
