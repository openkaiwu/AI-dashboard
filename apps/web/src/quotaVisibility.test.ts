import {describe, expect, it} from 'vitest';
import {renderToStaticMarkup} from 'react-dom/server';
import {createElement} from 'react';
import {MemoryRouter} from 'react-router-dom';
import CodexPlanner from './pages/CodexPlanner';
import {visibleQuotaWindows} from './quotaVisibility';

const metrics = [
  {bucket:'codex',slot:'secondary',duration_minutes:300,remaining:72,hours_left:3,resets_at:1900000000},
  {bucket:'codex',slot:'primary',duration_minutes:10080,remaining:86,hours_left:120,resets_at:1900400000},
];
describe('subscription quota visibility', () => {
  it.each(['pro', 'prolite', 'unknown'])('hides five-hour quotas for %s, preserves weekly quotas and raw data', plan => {
    expect(visibleQuotaWindows(metrics, plan).map(m => m.duration_minutes)).toEqual([10080]);
    expect(metrics).toHaveLength(2);
  });
  it('preserves both reported windows for Plus', () => {
    expect(visibleQuotaWindows(metrics, 'plus')).toEqual(metrics);
  });
  it.each(['pro', 'plus'])('applies %s visibility to budgets and reset events', planType => {
    const markup = renderToStaticMarkup(createElement(MemoryRouter, null,
      createElement(CodexPlanner, {planType, stale:false, device:{analysis:{metrics},snapshot:{},history:[]}})));
    expect(markup.includes('5 小时窗口')).toBe(planType === 'plus');
    expect(markup).toContain('168 小时窗口');
  });
});
