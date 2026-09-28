-- Extend Codex bridge rows with provider dimension; enable Cursor connector capability.
ALTER TABLE codex_bridges ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT 'codex';
UPDATE codex_bridges SET provider = 'codex' WHERE provider IS NULL OR provider = '';

UPDATE provider_capabilities
SET acquisition_mode = 'connector', connector_version = 'm0-1', support_level = 'partial'
WHERE provider_id = 'prov_cursor' AND capability = 'quota.pull';

CREATE INDEX IF NOT EXISTS provider_accounts_user_provider_idx ON provider_accounts(user_id, provider_id);
