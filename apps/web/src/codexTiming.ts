// Keep collection, polling and stale-state guidance aligned across clients.
export const CODEX_POLL_MS = 5 * 60 * 1000;
export const CODEX_STALE_MS = 11 * 60 * 1000;
/** Tibo / reset-radar check validity; matches server codex.NewsFreshFor. */
export const CODEX_NEWS_STALE_MS = 3 * 60 * 60 * 1000;
