-- Hourly usage rollups backing the daily/weekly/monthly consumption views (R5).
-- Raw codex_history is pruned after 7 days; these aggregates extend retention.
CREATE TABLE IF NOT EXISTS codex_usage_rollup (
    bridge_id   TEXT NOT NULL REFERENCES codex_bridges(id),
    hour_bucket TIMESTAMPTZ NOT NULL,
    window_key  TEXT NOT NULL,
    consumed_pp NUMERIC NOT NULL DEFAULT 0,
    samples     INTEGER NOT NULL DEFAULT 0,
    resets      INTEGER NOT NULL DEFAULT 0,
    level_last  NUMERIC,
    PRIMARY KEY (bridge_id, hour_bucket, window_key)
);

-- Marker so the consumption endpoint knows when raw history outruns the rollup
-- and a replay (self-healing backfill) is due.
CREATE TABLE IF NOT EXISTS codex_usage_rollup_state (
    bridge_id        TEXT PRIMARY KEY REFERENCES codex_bridges(id),
    last_observed_at TIMESTAMPTZ NOT NULL DEFAULT to_timestamp(0)
);
