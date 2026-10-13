export type UsageRow = {bucket_start:string;consumed_pp:number|null;coverage_hours:number;resets?:number|null};
export type HourRow = {hour:string;consumed_pp:number|null;samples:number;resets?:number};
export type UsageData = {
  generated_at?:string; tz_offset_minutes?:number;
  series?:UsageRow[]; hourly?:HourRow[];
  generations?:{resets_at:number|null;points:{t:string;used:number}[]}[];
};
export type UsageResource = {data?:UsageData;loading:boolean;error:boolean};
export type UsageResources = Record<'hourly'|'daily'|'five',UsageResource>;
export type ChartPoint = {key:string;label:string;value:number|null;resets:number};
export const emptyResources = ():UsageResources => ({hourly:{loading:true,error:false},daily:{loading:true,error:false},five:{loading:true,error:false}});
const valid = (v:unknown):v is number => typeof v==='number' && Number.isFinite(v) && v>=0;
export const sampleTotal = (points:ChartPoint[]):number|null => {
  const samples=points.filter(p=>p.value!==null);
  return samples.length?samples.reduce((sum,p)=>sum+p.value!,0):null;
};

// Preserve calendar gaps; seven observed days are not necessarily the last week.
export function dailyPoints(data:UsageData|undefined, days:number, now=Date.now()):ChartPoint[] {
  const shifted=new Date(now+(data?.tz_offset_minutes??0)*60000);
  const today=Date.UTC(shifted.getUTCFullYear(),shifted.getUTCMonth(),shifted.getUTCDate());
  const rows=new Map((data?.series||[]).map(row=>[row.bucket_start,row]));
  return Array.from({length:days},(_,i)=>{
    const key=new Date(today-(days-i-1)*86400000).toISOString().slice(0,10),row=rows.get(key);
    return {key,label:key.slice(5).replace('-','/'),value:row&&row.coverage_hours>0&&valid(row.consumed_pp)?row.consumed_pp:null,resets:row?.resets||0};
  });
}

// Merge rows for the same hour and retain empty hours as gaps in the chart.
export function hourlyPoints(data:UsageData|undefined, now=Date.now()):ChartPoint[] {
  const end=Math.floor(now/3600000)*3600000;
  const rows=new Map<number,{value:number;resets:number}>();
  for(const row of data?.hourly||[]) {
    if(!valid(row.consumed_pp)||row.samples<=0)continue;
    const hour=Math.floor(Date.parse(row.hour)/3600000)*3600000;
    if(!Number.isFinite(hour)||hour>end||hour<end-23*3600000)continue;
    const prev=rows.get(hour);
    rows.set(hour,{value:(prev?.value||0)+row.consumed_pp,resets:(prev?.resets||0)+(row.resets||0)});
  }
  return Array.from({length:24},(_,i)=>{
    const hour=end-(23-i)*3600000,row=rows.get(hour);
    const local=new Date(hour+(data?.tz_offset_minutes??0)*60000);
    return {key:new Date(hour).toISOString(),label:String(local.getUTCHours()).padStart(2,'0')+':00',value:row?.value??null,resets:row?.resets??0};
  });
}

export function latestFive(data:UsageData|undefined) {
  const generations=(data?.generations||[]).filter(g=>g.points?.some(p=>valid(p.used)&&p.used<=100&&Number.isFinite(Date.parse(p.t))));
  const generation=generations.at(-1);
  const point=generation?.points.filter(p=>valid(p.used)&&p.used<=100&&Number.isFinite(Date.parse(p.t))).at(-1);
  return point?{remaining:100-point.used,observed:point.t,resetsAt:generation!.resets_at}:null;
}

export function settleResource(previous:UsageResource,data:UsageData|undefined):UsageResource {
  return data?{data,loading:false,error:false}:{data:previous.data,loading:false,error:true};
}
