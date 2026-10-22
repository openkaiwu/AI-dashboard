export type RadarNews={checked_at:string;status:string;last_success_at?:string;items?:{url:string;published_at:string;summary:string;approximate_time?:boolean}[]};
export function radarItems(news:RadarNews|undefined,now:number,recent:boolean,search:string){
 const seen=new Set<string>();
 return (news?.items||[]).filter(item=>{
  const at=Date.parse(item.published_at);
  if(!/^https:\/\/x\.com\/thsottiaux\/status\/\d+$/.test(item.url)||seen.has(item.url)||!Number.isFinite(at)||at>now+120000)return false;
  seen.add(item.url);return (!recent||now-at<=72*3600000)&&(!search||item.summary.toLowerCase().includes(search.toLowerCase()));
 }).sort((a,b)=>Date.parse(b.published_at)-Date.parse(a.published_at));
}
export function quotaRecoveries(history:any[]){
 const result:{at:string;bucket:string;duration:number;before:number;after:number;cycleChanged:boolean}[]=[];
 const samples=[...history].filter(s=>s.status==='ok').sort((a,b)=>Date.parse(a.observed_at)-Date.parse(b.observed_at));
 for(let i=1;i<samples.length;i++){
  const prev=samples[i-1],cur=samples[i];const gap=Date.parse(cur.observed_at)-Date.parse(prev.observed_at);
  if(gap<=0||gap>15*60000)continue;
  for(const b of cur.buckets||[])for(const slot of ['primary','secondary']){
   const w=b[slot],old=prev.buckets?.find((x:any)=>x.id===b.id)?.[slot];
   if(!w||!old||w.duration_minutes!==old.duration_minutes||old.used_percent-w.used_percent<=2)continue;
   result.push({at:cur.observed_at,bucket:b.id,duration:w.duration_minutes,before:100-old.used_percent,after:100-w.used_percent,cycleChanged:w.resets_at!=null&&old.resets_at!=null&&w.resets_at!==old.resets_at});
  }
 }
 return result.reverse().slice(0,10);
}
