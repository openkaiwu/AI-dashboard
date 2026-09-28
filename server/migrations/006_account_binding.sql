-- Existing identities require an administrator decision before access is restored.
ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'member';
ALTER TABLE users ADD COLUMN IF NOT EXISTS account_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('admin','member'));
ALTER TABLE users ADD CONSTRAINT users_status_check CHECK (account_status IN ('pending','active','disabled'));

-- Old sessions have no trustworthy device kind or installation identity.
UPDATE devices SET revoked_at=now() WHERE revoked_at IS NULL;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'legacy';
ALTER TABLE devices ADD COLUMN IF NOT EXISTS installation_hash TEXT;
ALTER TABLE devices ADD CONSTRAINT devices_kind_check CHECK (kind IN ('legacy','desktop','mobile','web_admin'));
CREATE UNIQUE INDEX IF NOT EXISTS devices_one_active_kind ON devices(user_id,kind)
 WHERE revoked_at IS NULL AND kind IN ('desktop','mobile');

-- Bridge credentials are coupled to the physical desktop binding.
ALTER TABLE codex_bridges ADD COLUMN IF NOT EXISTS device_id TEXT REFERENCES devices(id);
UPDATE codex_bridges SET revoked_at=now() WHERE revoked_at IS NULL AND device_id IS NULL;
CREATE INDEX IF NOT EXISTS codex_bridges_device ON codex_bridges(device_id);
