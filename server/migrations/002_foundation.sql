DROP TABLE IF EXISTS sessions;
CREATE TABLE devices (
 id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id),
 name TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), revoked_at TIMESTAMPTZ
);
CREATE INDEX devices_user ON devices(user_id);
CREATE TABLE sessions (
 access_hash TEXT PRIMARY KEY, refresh_hash TEXT NOT NULL UNIQUE,
 user_id TEXT NOT NULL REFERENCES users(id), device_id TEXT NOT NULL REFERENCES devices(id),
 access_expires TIMESTAMPTZ NOT NULL, refresh_expires TIMESTAMPTZ NOT NULL,
 consumed BOOLEAN NOT NULL DEFAULT false, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX sessions_device ON sessions(device_id);
CREATE TABLE audit_events (
 id BIGSERIAL PRIMARY KEY, user_id TEXT NOT NULL, device_id TEXT NOT NULL,
 action TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Lock this row BEFORE allocating sequence, hold until commit: no BIGSERIAL commit-order holes.
CREATE TABLE sync_streams (
 user_id TEXT PRIMARY KEY REFERENCES users(id), epoch TEXT NOT NULL,
 seq BIGINT NOT NULL DEFAULT 0
);
CREATE TABLE sync_notes (
 user_id TEXT NOT NULL REFERENCES users(id), id TEXT NOT NULL,
 version BIGINT NOT NULL, deleted BOOLEAN NOT NULL DEFAULT false,
 payload JSONB NOT NULL, changed_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(user_id,id)
);
CREATE TABLE sync_events (
 user_id TEXT NOT NULL REFERENCES users(id), seq BIGINT NOT NULL,
 event JSONB NOT NULL, PRIMARY KEY(user_id,seq)
);
CREATE TABLE applied_operations (
 user_id TEXT NOT NULL REFERENCES users(id), operation_id TEXT NOT NULL,
 request_hash TEXT NOT NULL, result JSONB NOT NULL,
 PRIMARY KEY(user_id,operation_id)
);
CREATE TABLE jobs (
 id BIGSERIAL PRIMARY KEY, kind TEXT NOT NULL, payload JSONB NOT NULL,
 available_at TIMESTAMPTZ NOT NULL DEFAULT now(), lease_until TIMESTAMPTZ,
 lease_token TEXT, attempts INTEGER NOT NULL DEFAULT 0, completed_at TIMESTAMPTZ
);
CREATE INDEX jobs_claim ON jobs(available_at) WHERE completed_at IS NULL;
