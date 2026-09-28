CREATE TABLE codex_history (
 bridge_id TEXT NOT NULL REFERENCES codex_bridges(id), observed_at TIMESTAMPTZ NOT NULL,
 snapshot JSONB NOT NULL, PRIMARY KEY(bridge_id,observed_at)
);
CREATE TABLE codex_preferences (user_id TEXT PRIMARY KEY REFERENCES users(id), settings JSONB NOT NULL);
CREATE TABLE codex_alerts (
 user_id TEXT NOT NULL REFERENCES users(id), id TEXT NOT NULL,
 payload JSONB NOT NULL, first_seen TIMESTAMPTZ NOT NULL DEFAULT now(),
 last_seen TIMESTAMPTZ NOT NULL DEFAULT now(), notified_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 dismissed BOOLEAN NOT NULL DEFAULT false, snoozed_until TIMESTAMPTZ, active BOOLEAN NOT NULL DEFAULT true,
 PRIMARY KEY(user_id,id)
);
