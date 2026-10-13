-- Acknowledgement belongs to the account, so all clients stop repeating it.
ALTER TABLE codex_alerts ADD COLUMN read_at TIMESTAMPTZ;
-- NULL means the event has not yet been armed (for example during quiet hours).
ALTER TABLE codex_alerts ALTER COLUMN notified_at DROP NOT NULL;
ALTER TABLE codex_alerts ALTER COLUMN notified_at DROP DEFAULT;
