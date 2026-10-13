import { expect, it } from 'vitest';
import { CODEX_COLLECTION_INTERVAL_SECONDS, CODEX_POLL_MS, CODEX_STALE_MS } from './codexTiming';

it('refreshes collector data every minute and marks missed updates stale after three cycles', () => {
  expect(CODEX_COLLECTION_INTERVAL_SECONDS).toBe(60);
  expect(CODEX_POLL_MS).toBe(60_000);
  expect(CODEX_STALE_MS).toBe(3 * CODEX_POLL_MS);
});
