import {describe,it,expect,vi} from 'vitest';
import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {MemoryRouter} from 'react-router-dom';
import {ConsumptionView} from './UsageTab';
import {dailyPoints,hourlyPoints,latestFive,sampleTotal,settleResource} from './consumption';
import type {UsageResources,UsageResource} from './consumption';
vi.mock('./api',()=>({request:vi.fn()}));
const now=Date.parse('2026-10-11T04:00:00Z');
const empty:UsageResource={data:{series:[],hourly:[],generations:[]},loading:false,error:false};
const failed:UsageResource={loading:false,error:true};
const render=(resources:UsageResources,planType='pro')=>renderToStaticMarkup(createElement(MemoryRouter,null,createElement(ConsumptionView,{resources,planType,basis:'10080',onBasis:()=>{},range:'7d',onRange:()=>{},onRefresh:()=>{},now})));
describe('consumption data integrity and states',()=>{
  it('never presents response generation time as observation time',()=>{
    const daily={...empty,data:{generated_at:'2099-12-31T12:00:00Z'}};
    const html=render({daily,hourly:empty,five:empty});
    expect(html).toContain('采集时间未知');
    expect(html).not.toContain('2099');
    expect(html).not.toContain('数据截至');
  });
  it('shows observation time and source without requiring generation time',()=>{
    const daily={...empty,data:{data_as_of:'2026-10-11T03:00:00Z',source:'server_rollup',stale_reason:'stale_source'}};
    const html=render({daily,hourly:empty,five:empty});
    expect(html).toContain('数据截至');
    expect(html).toContain('服务器采样');
    expect(html).toContain('数据已滞后');
  });
  it('does not convert missing samples into zero',()=>{
    expect(sampleTotal(dailyPoints(undefined,7,now))).toBeNull();
    const points=dailyPoints({series:[{bucket_start:'2026-10-11',consumed_pp:0,coverage_hours:1}]},7,now);
    expect(sampleTotal(points)).toBe(0);
    expect(points.filter(p=>p.value!==null)).toHaveLength(1);
  });
  it('uses the last seven calendar days, not the last seven observed days',()=>{
    const points=dailyPoints({series:[{bucket_start:'2026-09-01',consumed_pp:90,coverage_hours:24},{bucket_start:'2026-10-05',consumed_pp:4,coverage_hours:1}]},7,now);
    expect(points[0].key).toBe('2026-10-05');
    expect(sampleTotal(points)).toBe(4);
  });
  it('respects account timezone near midnight',()=>{
    const points=dailyPoints({tz_offset_minutes:480,series:[]},7,Date.parse('2026-10-10T17:00:00Z'));
    expect(points.at(-1)!.key).toBe('2026-10-11');
  });
  it('aggregates duplicate hours and preserves missing hours',()=>{
    const points=hourlyPoints({hourly:[{hour:'2026-10-11T03:00:00Z',consumed_pp:2,samples:2},{hour:'2026-10-11T03:00:00Z',consumed_pp:3,samples:1},{hour:'2026-10-10T01:00:00Z',consumed_pp:90,samples:1},{hour:'2026-10-11T04:00:00Z',consumed_pp:0,samples:0}]},now);
    expect(points).toHaveLength(24);
    expect(sampleTotal(points)).toBe(5);
    expect(points.at(-1)!.value).toBeNull();
  });
  it('ignores empty five-hour generations safely',()=>{
    expect(latestFive({generations:[{resets_at:null,points:[]}]})).toBeNull();
  });
  it('retains last successful samples after a refresh failure',()=>{
    const prior={...empty,data:{series:[{bucket_start:'2026-10-11',consumed_pp:3,coverage_hours:1}],generated_at:'2026-10-11T03:00:00Z'}};
    const stale=settleResource(prior,undefined);
    expect(stale.data).toBe(prior.data);
    const html=render({daily:stale,hourly:empty,five:empty});
    expect(html).toContain('刷新失败，显示上次成功获取的数据');
    expect(html).toContain('3.0');
  });
  it('renders a single useful error without fake zero totals or charts',()=>{
    const html=render({daily:failed,hourly:failed,five:failed});
    expect(html).toContain('暂时无法获取消耗数据');
    expect(html).not.toContain('0.0');
    expect(html).not.toContain('consumption-plot');
    expect(html).toContain('检查电脑连接');
  });
  it('separates a successful empty response from failure',()=>{
    const html=render({daily:empty,hourly:empty,five:empty});
    expect(html).toContain('这段时间还没有消耗样本');
    expect(html).not.toContain('0.0');
    expect(html).not.toContain('consumption-plot');
  });
  it('keeps valid daily data visible when hourly data fails',()=>{
    const daily={...empty,data:{series:[{bucket_start:'2026-10-11',consumed_pp:7,coverage_hours:1}]}};
    const html=render({daily,hourly:failed,five:empty});
    expect(html).toContain('部分数据更新失败');
    expect(html).toContain('7.0');
    expect(html).toContain('consumption-plot');
  });
  it('only Plus gets a five-hour panel; unknown plan links to settings',()=>{
    const resources={daily:empty,hourly:empty,five:empty};
    expect(render(resources,'pro')).not.toContain('5 小时窗口');
    expect(render(resources,'plus')).toContain('5 小时窗口');
    expect(render(resources,'unknown')).toContain('/codex?tab=settings');
  });
});
