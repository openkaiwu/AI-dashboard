CREATE TABLE billing_events (
 id TEXT PRIMARY KEY,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 provider_account_id TEXT NOT NULL REFERENCES provider_accounts(id) ON DELETE CASCADE,
 event_type TEXT NOT NULL CHECK (event_type IN ('charge','credit','refund','adjustment')),
 amount REAL NOT NULL CHECK (amount >= 0),
 currency TEXT NOT NULL,
 source_type TEXT NOT NULL,
 observed_at TIMESTAMPTZ NOT NULL,
 note TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX billing_events_account_time ON billing_events(provider_account_id, observed_at DESC);
