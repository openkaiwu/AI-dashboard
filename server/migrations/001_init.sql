CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    token TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS sessions_user_idx ON sessions(user_id);

CREATE TABLE IF NOT EXISTS providers (
    id TEXT PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    category TEXT NOT NULL,
    homepage TEXT NOT NULL,
    metadata_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS provider_capabilities (
    provider_id TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    capability TEXT NOT NULL,
    support_level TEXT NOT NULL,
    acquisition_mode TEXT NOT NULL,
    connector_version TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (provider_id, capability, acquisition_mode)
);

CREATE TABLE IF NOT EXISTS provider_accounts (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL REFERENCES providers(id),
    display_name TEXT NOT NULL,
    external_account_hint TEXT NOT NULL DEFAULT '',
    region TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS provider_accounts_user_idx ON provider_accounts(user_id);

CREATE TABLE IF NOT EXISTS entitlements (
    id TEXT PRIMARY KEY,
    provider_account_id TEXT NOT NULL REFERENCES provider_accounts(id) ON DELETE CASCADE,
    plan_code TEXT NOT NULL DEFAULT '',
    plan_name TEXT NOT NULL,
    starts_at TEXT,
    renews_at TEXT,
    expires_at TEXT,
    currency TEXT NOT NULL DEFAULT '',
    price REAL,
    source_type TEXT NOT NULL DEFAULT 'user_manual',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS entitlements_account_idx ON entitlements(provider_account_id);

CREATE TABLE IF NOT EXISTS quota_buckets (
    id TEXT PRIMARY KEY,
    provider_account_id TEXT NOT NULL REFERENCES provider_accounts(id) ON DELETE CASCADE,
    entitlement_id TEXT REFERENCES entitlements(id) ON DELETE SET NULL,
    scope_key TEXT NOT NULL DEFAULT 'default',
    quota_type TEXT NOT NULL,
    unit TEXT NOT NULL,
    limit_value REAL,
    reset_policy TEXT NOT NULL DEFAULT 'unknown',
    reset_at TEXT,
    expires_at TEXT,
    rolling_window_seconds INTEGER,
    source_type TEXT NOT NULL DEFAULT 'user_manual',
    confidence TEXT NOT NULL DEFAULT 'high',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS quota_buckets_account_idx ON quota_buckets(provider_account_id);

CREATE TABLE IF NOT EXISTS usage_snapshots (
    id TEXT PRIMARY KEY,
    quota_bucket_id TEXT NOT NULL REFERENCES quota_buckets(id) ON DELETE CASCADE,
    observed_at TEXT NOT NULL,
    used_value REAL,
    remaining_value REAL,
    remaining_ratio REAL,
    note TEXT NOT NULL DEFAULT '',
    raw_value_json TEXT NOT NULL DEFAULT '{}',
    source_type TEXT NOT NULL DEFAULT 'user_manual',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS usage_snapshots_bucket_time_idx ON usage_snapshots(quota_bucket_id, observed_at);

CREATE TABLE IF NOT EXISTS notification_rules (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    rule_type TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    params_json TEXT NOT NULL,
    provider_account_id TEXT REFERENCES provider_accounts(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS notification_rules_user_idx ON notification_rules(user_id);

CREATE TABLE IF NOT EXISTS notifications (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rule_id TEXT REFERENCES notification_rules(id) ON DELETE SET NULL,
    provider_account_id TEXT,
    quota_bucket_id TEXT,
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'info',
    dedupe_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'unread',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS notifications_user_time_idx ON notifications(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS alert_states (
    user_id TEXT NOT NULL,
    dedupe_key TEXT NOT NULL,
    notification_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (user_id, dedupe_key)
);
