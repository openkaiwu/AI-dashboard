-- M5 workspace & collaboration v1 (INH-501/503/509): workspaces scope shared canonical assets only.
CREATE TABLE workspaces (
 id TEXT PRIMARY KEY,
 name TEXT NOT NULL,
 owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE workspace_members (
 workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 role TEXT NOT NULL CHECK (role IN ('owner','editor','viewer')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY (workspace_id, user_id)
);
CREATE INDEX workspace_members_user ON workspace_members(user_id);

CREATE TABLE workspace_invites (
 id TEXT PRIMARY KEY,
 workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 email TEXT NOT NULL,
 role TEXT NOT NULL CHECK (role IN ('editor','viewer')),
 status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','accepted','revoked')),
 created_by TEXT NOT NULL REFERENCES users(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 accepted_at TIMESTAMPTZ,
 UNIQUE (workspace_id, email)
);

CREATE TABLE workspace_comments (
 id TEXT PRIMARY KEY,
 workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 target_type TEXT NOT NULL CHECK (target_type IN ('conversation','config_asset')),
 target_id TEXT NOT NULL,
 body TEXT NOT NULL,
 mention_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX workspace_comments_target ON workspace_comments(workspace_id, target_type, target_id, created_at);

-- Explicit per-resource sharing (frozen v1): the owner binds a resource to a workspace.
ALTER TABLE conversations ADD COLUMN workspace_id TEXT REFERENCES workspaces(id) ON DELETE SET NULL;
ALTER TABLE config_assets ADD COLUMN workspace_id TEXT REFERENCES workspaces(id) ON DELETE SET NULL;
CREATE INDEX conversations_workspace ON conversations(workspace_id) WHERE workspace_id IS NOT NULL;
CREATE INDEX config_assets_workspace ON config_assets(workspace_id) WHERE workspace_id IS NOT NULL;
