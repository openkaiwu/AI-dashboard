CREATE TABLE connector_samples (
 id TEXT PRIMARY KEY,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
 provider_slug TEXT NOT NULL,
 sample_kind TEXT NOT NULL,
 used_percent REAL NOT NULL,
 content_hash TEXT NOT NULL,
 observed_at TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE (device_id, provider_slug, content_hash)
);
CREATE INDEX connector_samples_user ON connector_samples(user_id, created_at DESC);
