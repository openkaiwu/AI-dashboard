import {describe, expect, it} from 'vitest';
import {localTimestamp, refreshWithCache} from './cacheRefresh';

describe('overview refresh', () => {
  it('keeps fresh data when storage is full', async () => {
    const result = await refreshWithCache(async () => ({remaining: 86}), () => {throw new Error('quota');});
    expect(result.data.remaining).toBe(86);
    expect(result.cacheError).toContain('最新数据已显示');
  });
  it('does not save a failed request', async () => {
    let writes = 0;
    await expect(refreshWithCache(async () => {throw new Error('offline');}, () => {writes++;})).rejects.toThrow('offline');
    expect(writes).toBe(0);
  });
  it('does not invent an observation time', () => {
    expect(localTimestamp(undefined, '采集时间未知')).toBe('采集时间未知');
    expect(localTimestamp('invalid', '采集时间未知')).toBe('采集时间未知');
  });
});
