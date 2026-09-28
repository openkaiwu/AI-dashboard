CREATE TABLE codex_plan (
 user_id TEXT PRIMARY KEY REFERENCES users(id),
 plan_type TEXT NOT NULL CHECK (plan_type IN ('plus','pro','unknown')),
 source_type TEXT NOT NULL CHECK (source_type IN ('manual','collector')),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
