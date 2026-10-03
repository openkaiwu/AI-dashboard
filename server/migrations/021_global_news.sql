-- Global reset-radar news promoted from the administrator's bridge snapshots (R5).
-- Single row by construction: the newest admin-verified check is the radar for every account.
CREATE TABLE IF NOT EXISTS global_codex_news (
    id INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    payload JSONB NOT NULL,
    source_user_id TEXT NOT NULL REFERENCES users(id),
    refreshed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
