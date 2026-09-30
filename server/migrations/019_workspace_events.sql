-- M5 completion (INH-509): workspace activity events (share/comment/membership/branch).
CREATE TABLE workspace_events (
 id TEXT PRIMARY KEY,
 workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 event_type TEXT NOT NULL,
 target_type TEXT NOT NULL DEFAULT '',
 target_id TEXT NOT NULL DEFAULT '',
 summary TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX workspace_events_feed ON workspace_events(workspace_id, created_at DESC);
