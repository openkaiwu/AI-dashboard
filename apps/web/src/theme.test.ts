import { describe, expect, it } from 'vitest';
import { parseTheme, resolveTheme } from './theme';
describe('appearance preference', () => {
  it('treats missing or corrupt stored values as system', () => {
    for (const value of [null, undefined, '', 'invalid', {}]) expect(parseTheme(value)).toBe('system');
  });
  it('follows OS changes only when requested', () => {
    expect(resolveTheme('system', true)).toBe('dark');
    expect(resolveTheme('system', false)).toBe('light');
    expect(resolveTheme('dark', false)).toBe('dark');
    expect(resolveTheme('light', true)).toBe('light');
  });
});
