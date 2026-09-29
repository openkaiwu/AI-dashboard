-- M4 portable config v1 (INH-459/INH-463): AIPortableConfig assets, append-only versions, secret-free payloads.
CREATE TABLE config_assets (
 id TEXT PRIMARY KEY,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 name TEXT NOT NULL,
 kind TEXT NOT NULL CHECK (kind IN ('mcp_server','prompt_template','agent_profile')),
 source_platform TEXT NOT NULL DEFAULT 'canonical',
 latest_version INTEGER NOT NULL DEFAULT 0,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX config_assets_user_time ON config_assets(user_id, updated_at DESC);

CREATE TABLE config_versions (
 id TEXT PRIMARY KEY,
 asset_id TEXT NOT NULL REFERENCES config_assets(id) ON DELETE CASCADE,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 version INTEGER NOT NULL,
 content JSONB NOT NULL,
 content_hash TEXT NOT NULL,
 loss_report JSONB NOT NULL DEFAULT '[]'::jsonb,
 created_by TEXT NOT NULL CHECK (created_by IN ('manual','import','transform','rollback')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE (asset_id, version)
);
CREATE INDEX config_versions_asset ON config_versions(asset_id, version DESC);

CREATE TABLE config_bindings (
 id TEXT PRIMARY KEY,
 asset_id TEXT NOT NULL REFERENCES config_assets(id) ON DELETE CASCADE,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 target_platform TEXT NOT NULL,
 target_path TEXT NOT NULL DEFAULT '',
 last_applied_version INTEGER NOT NULL DEFAULT 0,
 status TEXT NOT NULL DEFAULT 'declared' CHECK (status IN ('declared','applied','failed')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE (asset_id, target_platform, target_path)
);

-- Bridge config discovery (INH-467): metadata and secret KEY NAMES only, never values.
CREATE TABLE config_discoveries (
 id TEXT PRIMARY KEY,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 device_id TEXT NOT NULL,
 path TEXT NOT NULL,
 platform TEXT NOT NULL,
 size_bytes BIGINT NOT NULL DEFAULT 0,
 modified_at TIMESTAMPTZ,
 secret_keys TEXT NOT NULL DEFAULT '',
 detected_format TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL DEFAULT 'discovered' CHECK (status IN ('discovered','imported','ignored')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE (device_id, path)
);
