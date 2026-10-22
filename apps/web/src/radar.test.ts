import {describe,it,expect} from 'vitest';
import {radarItems,quotaRecoveries} from './radar';
describe('radar evidence',()=>{
 it('deduplicates sources, rejects unsupported links and filters old news',()=>{
  const now=Date.now(),item={url:'https://x.com/thsottiaux/status/123',published_at:new Date(now).toISOString(),summary:'reset'};
  expect(radarItems({checked_at:item.published_at,status:'checked',items:[item,item,{...item,url:'https://example.com'}]},now,true,'')).toEqual([item]);
 });
 it('does not infer a reset through a long collection gap',()=>{
  const sample=(t:number,used:number)=>({status:'ok',observed_at:new Date(t).toISOString(),buckets:[{id:'x',primary:{used_percent:used,duration_minutes:300,resets_at:100}}]});
  expect(quotaRecoveries([sample(0,90),sample(3600000,1)])).toEqual([]);
  expect(quotaRecoveries([sample(0,90),sample(60000,1)])[0].cycleChanged).toBe(false);
 });
});
