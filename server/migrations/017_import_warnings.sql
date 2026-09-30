-- M3 completion: explicit import warnings (INH-446 acceptance: no silent field loss).
ALTER TABLE conversation_imports ADD COLUMN warnings TEXT NOT NULL DEFAULT '';
ALTER TABLE conversation_raw_snapshots ADD COLUMN warnings TEXT NOT NULL DEFAULT '';
