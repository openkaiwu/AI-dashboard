import {useEffect,useState} from "react";
import {request} from "./api";
import {friendlyError} from "./errors";
type UsageSeries={bucket_start:string;consumed_pp:number|null;level_end:number|null;peak_pp_hour:number|null;resets:number|null;coverage_hours:number};
type UsageGeneration={resets_at:number|null;start:string;end:string;points:{t:string;used:number}[]};
type UsageWindow={id:string;duration_minutes:number;present:boolean};
type Hourly={hour:string;consumed_pp:number;samples:number;resets:number};
type Tip={x:number;y:number;lines:string[]};
type UsageData={generated_at:string;tz_offset_minutes:number;windows?:UsageWindow[];series?:UsageSeries[];generations?:UsageGeneration[];coverage?:{first_day:string;days_with_data:number};reference_pp?:number;plan_type?:string;compare?:{this_pp:number;prev_pp:number;ratio?:number}};
const DUR_LABEL:Record<number,string>={300:"5 小时窗口",10080:"7 天窗口"};
const PLAN_LABEL:Record<string,string>={plus:"Plus",pro:"Pro",prolite:"Pro Lite",unknown:"套餐未确认"};
const when=(v:string)=>new Date(v).toLocaleString();
const sum=(a:UsageSeries[])=>a.reduce((x,s)=>x+(s.consumed_pp||0),0);
export default function UsageTab(){
 const[loading,setLoading]=useState(true);
 const[d300,setD300]=useState<UsageData|null>(null);
 const[w10080,setW10080]=useState<UsageData|null>(null);
 const[h24,setH24]=useState<{hourly?:Hourly[];plan_type?:string}|null>(null);
 const[d30,setD30]=useState<UsageData|null>(null);
 const[wk,setWk]=useState<UsageData|null>(null);
 const[mo,setMo]=useState<UsageData|null>(null);
 const[error,setError]=useState("");
 const[tip,setTip]=useState<Tip|null>(null);
 useEffect(()=>{void reloadAll();},[]);
 async function reloadAll(){
  setLoading(true);
  const r=(g:string,extra="")=>request<UsageData>(`/api/v1/codex/consumption?granularity=${g}${extra}`).catch(()=>null);
  const[a,b,c,d,e,f]=await Promise.all([r("window","&window=300"),r("window","&window=10080"),r("hourly","&hours=24"),r("daily","&days=30"),r("weekly","&compare=true"),r("monthly")]);
  setD300(a);setW10080(b);setH24(c);setD30(d);setWk(e);setMo(f);
  setError(a||b||c||d||e||f?"":"消耗数据暂不可用，请稍后重试。");
  setLoading(false);
 }
 const planType=d300?.plan_type||w10080?.plan_type||d30?.plan_type||"unknown";
 const fiveOK=(planType==="plus"||planType==="pro")&&!!d300?.windows?.find(w=>w.duration_minutes===300&&w.present);
 const daily=(d30?.series||[]).filter(s=>s.consumed_pp!=null);
 const last7=daily.slice(-7);
 const last24=(h24?.hourly||[]).reduce((a,h)=>a+(h.consumed_pp||0),0);
 const wGen=(w10080?.generations||[]);
 const wCur=wGen.length>0?wGen[wGen.length-1].points[wGen[wGen.length-1].points.length-1]:null;
 const hourly=(h24?.hourly||[]);
 const cmp=wk?.compare;
 return <section className="panel">
  <div className="section-heading"><h2>消耗分析</h2><button className="btn secondary" onClick={()=>void reloadAll()}>↻ 刷新</button></div>
  <p className="muted">四个时间维度的额度消耗：五小时窗口实时状态、最近 24 小时、最近 7 天与最近 30 天。重置不算消耗，空档表示无样本。</p>
  {loading&&<><div className="skeleton" style={{height:90}}/><div className="skeleton" style={{height:150,marginTop:12}}/><div className="skeleton" style={{height:150,marginTop:12}}/><div className="skeleton" style={{height:150,marginTop:12}}/></>}
  {!loading&&error&&<p className="error" role="alert">{error} <button className="btn ghost" onClick={()=>void reloadAll()}>重试</button></p>}
  {!loading&&<>
  <article className="device-card">
   <h2>⏱ 五小时窗口（实时）{data?.plan_type&&<span className="device-tag" style={{marginLeft:8}}>{PLAN_LABEL[planType]||planType}</span>}</h2>
   {fiveOK&&d300&&d300.generations&&d300.generations.length>0?(()=>{
    const gens=d300.generations;
    const cur=gens[gens.length-1];
    const last=cur.points[cur.points.length-1];
    const remaining=100-last.used;
    const prev=cur.points.length>=2?cur.points[cur.points.length-2]:null;
    const pace=prev?Math.max(0,(last.used-prev.used)/Math.max(1,(new Date(last.t).getTime()-new Date(prev.t).getTime())/36e5)):0;
    const proj=pace>0.5?new Date(new Date(last.t).getTime()+((100-last.used)/pace)*36e5).toLocaleTimeString():"速率平稳";
    const resetAt=cur.resets_at?new Date(cur.resets_at*1000):null;
    const runMin=resetAt?Math.round((Date.now()-(resetAt.getTime()-300*6e4))/6e4):null;
    return <><div className="quota-number">{remaining.toFixed(0)}<span>% 剩余</span></div>
    <WindowGenerations gens={gens}/>
    <p className="muted compact">已运行 {runMin!=null?`${Math.floor(runMin/60)} 小时 ${runMin%60} 分`:"—"} / 5 小时 · 速率 {pace.toFixed(1)} pp/小时 · 预计触顶 {proj}{resetAt?` · 重置于 ${when(resetAt.toISOString())}`:""}</p></>;
   })():<p className="muted">{planType==="unknown"?"请先在「使用计划」选择套餐以启用五小时视图。":"当前套餐 Codex 未上报五小时窗口（以 Codex 实际显示为准）。"}</p>}
  </article>
  <article className="device-card">
   <h2>📅 一天</h2>
   <div className="quota-number">{last24.toFixed(1)}<span>pp / 近 24 小时</span></div>
   {hourly.length>0&&<MiniBars rows={hourly}/>}
   <BarChart series={daily} scale={Math.max(100,sum(daily))} granularity="daily" reference={d30&&d30.reference_pp!=null?d30.reference_pp:0} onTip={setTip} clearTip={()=>setTip(null)}/>
   <p className="muted compact">昨日 {yesterdaySum(daily)} pp · 30 天日均 {(sum(daily)/Math.max(1,daily.length)).toFixed(1)} pp · 峰值 {peakOf(daily).toFixed(1)} pp</p>
  </article>
  <article className="device-card">
   <h2>🗓 一周</h2>
   <div className="quota-number">{last7sum.toFixed(1)}<span>pp / 近 7 天</span></div>
   <BarChart series={last7} scale={Math.max(100,last7sum)} granularity="weekly" reference={0} onTip={setTip} clearTip={()=>setTip(null)}/>
   {cmp&&<p className="muted compact">本日历周 {cmp.this_pp.toFixed(1)} pp · 上周同期 {cmp.prev_pp.toFixed(1)} pp{cmp.ratio!=null?` · 环比 ${(cmp.ratio*100).toFixed(0)}%`:""}</p>}
   {wCur&&<p className="muted compact">周窗口（Codex 原生）：已用 {wCur.used.toFixed(0)}% · 与"近 7 天消耗"口径不同，两者并列仅供参考。</p>}
  </article>
  <article className="device-card">
   <h2>📆 一月</h2>
   <div className="quota-number">{sum(daily).toFixed(1)}<span>pp / 近 30 天</span></div>
   <BarChart series={daily} scale={Math.max(100,sum(daily))} granularity="monthly" reference={0} onTip={setTip} clearTip={()=>setTip(null)}/>
   {mo?.series&&mo.series.length>0&&<p className="muted compact">月度合计：{mo.series.map(m=>`${m.bucket_start.slice(0,7)} ${m.consumed_pp==null?"—":m.consumed_pp.toFixed(1)+" pp"}`).join(" · ")}（自部署起积累）</p>}
   <p className="muted compact">30 天日均 {(sum(daily)/Math.max(1,daily.length)).toFixed(1)} pp · 覆盖 {daily.length} 天。</p>
  </article>
  </>}
  {!loading&&tip&&<div style={{position:"fixed",left:tip.x+14,top:tip.y-12,pointerEvents:"none",background:"var(--panel)",border:"1px solid var(--line)",borderRadius:8,padding:"6px 10px",fontSize:12,zIndex:9,boxShadow:"var(--shadow)"}}>{tip.lines.map((l,i)=><div key={i}>{l}</div>)}</div>}
 </section>;
}
function yesterdaySum(daily:UsageSeries[]){
 const y=new Date(Date.now()-864e5).toLocaleDateString("sv");
 const row=daily.find(s=>s.bucket_start===y);
 return row&&row.consumed_pp!=null?row.consumed_pp:0;
}
function peakOf(daily:UsageSeries[]){
 return daily.reduce((a,s)=>Math.max(a,s.consumed_pp||0),0);
}
function MiniBars({rows}:{rows:Hourly[]}){
 const scale=Math.max(1,...rows.map(r=>r.consumed_pp));
 const bw=Math.max(3,580/Math.max(1,rows.length)-2);
 return <div className="trend"><svg viewBox="0 0 600 90" role="img" aria-label="最近 24 小时逐小时消耗">{rows.map((r,i)=>{const cx=10+i*(580/rows.length)+bw/2;return <g key={r.hour}><title>{`${when(r.hour)}：${r.consumed_pp.toFixed(1)} pp`}</title><rect x={cx-bw/2} y={80-r.consumed_pp/scale*70} width={bw} height={Math.max(1,r.consumed_pp/scale*70)} fill="var(--accent)" opacity=".85"/></g>;})}</svg><div className="trend-axis"><span>{rows.length?when(rows[0].hour):""}</span><span>逐小时 pp</span><span>{rows.length?when(rows[rows.length-1].hour):""}</span></div></div>;
}
function WindowGenerations({gens}:{gens:UsageGeneration[]}){
 const all=gens.flatMap(g=>g.points);
 const start=Math.min(...all.map(p=>new Date(p.t).getTime())),end=Math.max(...all.map(p=>new Date(p.t).getTime()));
 const x=(t:string)=>10+(new Date(t).getTime()-start)/Math.max(1,end-start)*580;
 const y=(u:number)=>95-u*.8;
 return <div className="trend"><svg viewBox="0 0 600 110" role="img" aria-label="当前窗口已用额度趋势，纵轴 0 到 100%"><path d="M10 15H590 M10 55H590 M10 95H590" stroke="currentColor" opacity=".1"/>{gens.map((g,gi)=>g.points.length>=2?<polyline key={gi} fill="none" stroke="var(--accent)" strokeWidth="2.5" points={g.points.map(p=>`${x(p.t)},${y(p.used)}`).join(" ")}/>:<circle key={gi} cx={x(g.points[0].t)} cy={y(g.points[0].used)} r="3" fill="var(--accent)"/>)}<text x="12" y="12">100%</text><text x="12" y="108">0%</text></svg><div className="trend-axis"><span>{when(gens[0].start)}</span><span>已用额度 · 当前窗口（重置处分段）</span><span>{when(gens[gens.length-1].end)}</span></div></div>;
}
function BarChart({series,scale,granularity,reference,onTip,clearTip}:{series:UsageSeries[];scale:number;granularity:string;reference:number;onTip:(t:Tip)=>void;clearTip:()=>void}){
 const bw=Math.max(2,580/Math.max(1,series.length)-4);
 return <div className="trend"><svg viewBox="0 0 600 140" role="img" aria-label={`每个${granularity==="daily"?"日":granularity==="weekly"?"周":"月"}的额度消耗，纵轴按 ${scale.toFixed(0)} pp 缩放`}>
  <path d="M10 20H590 M10 70H590 M10 120H590" stroke="currentColor" opacity=".1"/>
  {reference>0&&reference<=scale&&<g><line x1="10" x2="590" y1={120-reference/scale*100} y2={120-reference/scale*100} stroke="var(--warn)" strokeDasharray="5 4" opacity=".8"/><text x="588" y={114-reference/scale*100} textAnchor="end" fontSize="10" fill="var(--warn)">参考 {reference.toFixed(1)} pp</text></g>}
  {series.map((s,i)=>{
   if(s.consumed_pp==null)return null;
   const used=s.consumed_pp,over=reference>0&&used>reference,cy=120-used/scale*100,cx=10+i*(580/series.length)+bw/2;
   return <g key={s.bucket_start} onMouseMove={e=>onTip({x:e.clientX,y:e.clientY,lines:[s.bucket_start,`消耗 ${used.toFixed(1)} pp`,s.resets?`重置 ${s.resets} 次`:"无重置",`覆盖 ${s.coverage_hours} 小时`]})} onMouseLeave={clearTip}><rect x={cx-bw/2} y={cy} width={bw} height={Math.max(1,used/scale*100)} fill={over?"var(--warn)":"var(--accent)"} opacity={s.coverage_hours===0?0.3:0.85}/>{s.resets?s.resets>0&&<text x={cx} y={cy-6} textAnchor="middle" fontSize="10">▲</text>:null}</g>;})}
  <text x="12" y="14">{scale.toFixed(0)} pp</text>
 </svg>
 <div className="trend-axis"><span>{series[0]?.bucket_start||""}</span><span>消耗（百分点）· ▲ 重置</span><span>{series[series.length-1]?.bucket_start||""}</span></div></div>;
}
