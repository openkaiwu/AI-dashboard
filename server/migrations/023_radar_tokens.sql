-- Long-lived token for the reset-radar automation to push checked news
-- directly (POST /api/v1/radar/news) without the desktop bridge online.
CREATE TABLE IF NOT EXISTS radar_tokens (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    note TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);
