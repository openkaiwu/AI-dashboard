// Keep collection, polling and stale-state guidance aligned across clients.
export const CODEX_POLL_MS = 60 * 1000;
export const CODEX_STALE_MS = 3 * 60 * 1000;
export const CODEX_COLLECTION_INTERVAL_SECONDS = 60;
/** Tibo / reset-radar check validity; matches server codex.NewsFreshFor. */
export const CODEX_NEWS_STALE_MS = 45 * 60 * 1000;
