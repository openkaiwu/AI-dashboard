CREATE TABLE codex_bridges (
 id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id), name TEXT NOT NULL,
 token_hash TEXT NOT NULL UNIQUE, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 revoked_at TIMESTAMPTZ, received_at TIMESTAMPTZ, snapshot JSONB
);
CREATE INDEX codex_bridges_user ON codex_bridges(user_id);
