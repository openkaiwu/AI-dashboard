-- M3 conversation portability v1 (INH-433/INH-438): canonical append-only graph.
CREATE TABLE projects (
 id TEXT PRIMARY KEY,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 name TEXT NOT NULL,
 description TEXT NOT NULL DEFAULT '',
 external_source TEXT NOT NULL DEFAULT '',
 external_id TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX projects_dedup ON projects(user_id, external_source, external_id) WHERE external_id <> '';
CREATE INDEX projects_user_time ON projects(user_id, created_at DESC);

CREATE TABLE conversations (
 id TEXT PRIMARY KEY,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 project_id TEXT REFERENCES projects(id) ON DELETE SET NULL,
 title TEXT NOT NULL,
 provider_slug TEXT NOT NULL,
 external_id TEXT NOT NULL DEFAULT '',
 dedup_key TEXT NOT NULL,
 content_hash TEXT NOT NULL,
 message_count INTEGER NOT NULL DEFAULT 0,
 imported_via TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX conversations_dedup ON conversations(user_id, provider_slug, dedup_key);
CREATE INDEX conversations_user_time ON conversations(user_id, updated_at DESC);

CREATE TABLE conversation_branches (
 id TEXT PRIMARY KEY,
 conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 name TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX conversation_branches_name ON conversation_branches(conversation_id, name);

CREATE TABLE conversation_messages (
 id TEXT PRIMARY KEY,
 conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 branch_id TEXT REFERENCES conversation_branches(id) ON DELETE CASCADE,
 parent_id TEXT REFERENCES conversation_messages(id) ON DELETE CASCADE,
 position INTEGER NOT NULL DEFAULT 0,
 role TEXT NOT NULL CHECK (role IN ('user','assistant','system','tool')),
 content TEXT NOT NULL,
 sent_at TIMESTAMPTZ,
 external_ref TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX conversation_messages_conv ON conversation_messages(conversation_id, branch_id, position);

CREATE TABLE conversation_imports (
 id TEXT PRIMARY KEY,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 source_type TEXT NOT NULL,
 file_name TEXT NOT NULL DEFAULT '',
 size_bytes INTEGER NOT NULL DEFAULT 0,
 status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','completed','failed')),
 conversations_created INTEGER NOT NULL DEFAULT 0,
 conversations_deduplicated INTEGER NOT NULL DEFAULT 0,
 messages_imported INTEGER NOT NULL DEFAULT 0,
 branches_created INTEGER NOT NULL DEFAULT 0,
 error TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 completed_at TIMESTAMPTZ
);
CREATE INDEX conversation_imports_user ON conversation_imports(user_id, created_at DESC);

CREATE TABLE conversation_raw_snapshots (
 id TEXT PRIMARY KEY,
 import_id TEXT NOT NULL REFERENCES conversation_imports(id) ON DELETE CASCADE,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 source_type TEXT NOT NULL,
 content TEXT NOT NULL,
 content_hash TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
