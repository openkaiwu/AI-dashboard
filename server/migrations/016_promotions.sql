-- M6 promotion intelligence v1 (INH-484/488/491/495): source registry, dedup, watchlists.
CREATE TABLE promotion_sources (
 id TEXT PRIMARY KEY,
 slug TEXT NOT NULL UNIQUE,
 kind TEXT NOT NULL CHECK (kind IN ('official_blog','pricing_page','announcement','rss','user_submit')),
 url TEXT NOT NULL DEFAULT '',
 trusted BOOLEAN NOT NULL DEFAULT FALSE,
 enabled BOOLEAN NOT NULL DEFAULT TRUE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE promotions (
 id TEXT PRIMARY KEY,
 content_hash TEXT NOT NULL UNIQUE,
 provider_slug TEXT NOT NULL DEFAULT '',
 plan TEXT NOT NULL DEFAULT '',
 region TEXT NOT NULL DEFAULT '',
 title TEXT NOT NULL,
 url TEXT NOT NULL,
 discount TEXT NOT NULL DEFAULT '',
 starts_at TIMESTAMPTZ,
 ends_at TIMESTAMPTZ,
 time_precision TEXT NOT NULL DEFAULT 'unknown' CHECK (time_precision IN ('exact','day','unknown')),
 confidence TEXT NOT NULL CHECK (confidence IN ('high','medium','low')),
 status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','expired')),
 source_id TEXT NOT NULL REFERENCES promotion_sources(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 archived_at TIMESTAMPTZ
);
CREATE INDEX promotions_status_time ON promotions(status, created_at DESC);
CREATE INDEX promotions_match ON promotions(provider_slug, status);

CREATE TABLE promotion_observations (
 promotion_id TEXT NOT NULL REFERENCES promotions(id) ON DELETE CASCADE,
 source_id TEXT NOT NULL REFERENCES promotion_sources(id) ON DELETE CASCADE,
 observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY (promotion_id, source_id)
);

CREATE TABLE promotion_watchlists (
 id TEXT PRIMARY KEY,
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 provider_slug TEXT NOT NULL DEFAULT '',
 plan TEXT NOT NULL DEFAULT '',
 region TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE (user_id, provider_slug, plan, region)
);

-- One notification per user per promotion: the dedup guarantee of INH-497.
CREATE TABLE promotion_notifications (
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 promotion_id TEXT NOT NULL REFERENCES promotions(id) ON DELETE CASCADE,
 notification_id TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY (user_id, promotion_id)
);
