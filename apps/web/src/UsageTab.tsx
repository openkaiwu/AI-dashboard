import {useEffect,useState} from "react";
import {request} from "./api";
import {friendlyError} from "./errors";
type UsageSeries={bucket_start:string;consumed_pp:number|null;level_end:number|null;peak_pp_hour:number|null;resets:number|null;coverage_hours:number};
type UsageGeneration={resets_at:number|null;start:string;end:string;points:{t:string;used:number}[]};
type UsageWindow={id:string;duration_minutes:number;present:boolean};
type Tip={x:number;y:number;lines:string[]};
type UsageData={generated_at:string;tz_offset_minutes:number;windows:UsageWindow[];series?:UsageSeries[];generations?:UsageGeneration[];coverage?:{first_day:string;days_with_data:number};reference_pp?:number};
const GRANULARITIES:[string,string][]=[["window","当前窗口"],["daily","每日"],["weekly","每周"],["monthly","每月"]];
const DUR_LABEL:Record<number,string>={300:"5 小时窗口",10080:"7 天窗口"};
const when=(v:string)=>new Date(v).toLocaleString();
export default function UsageTab(){
 const[granularity,setGranularity]=useState("daily");
 const[windowFilter,setWindowFilter]=useState("");
 const[days,setDays]=useState(0);
 const[data,setData]=useState<UsageData|null>(null);
 const[error,setError]=useState(""),[loading,setLoading]=useState(true);
 const[tip,setTip]=useState<Tip|null>(null);
 useEffect(()=>{void reload(granularity,windowFilter,days);},[granularity,windowFilter,days]);
 async function reload(g:string,w:string,d=days){
  setLoading(true);
  try{setData(await request<UsageData>(`/api/v1/codex/consumption?granularity=${g}${w?`&window=${w}`:""}${d?`&days=${d}`:""}`));setError("");}
  catch(e){setError(friendlyError(e));}
  finally{setLoading(false);}
 }
 const series=(data?.series||[]).filter(s=>s.consumed_pp!=null);
 const total=series.reduce((a,s)=>a+(s.consumed_pp||0),0);
 const peak=series.reduce((a,s)=>Math.max(a,s.consumed_pp||0),0);
 const resets=series.reduce((a,s)=>a+(s.resets||0),0);
 const unit=granularity==="daily"?"日":granularity==="weekly"?"周":"月";
 return <section className="panel">
  <div className="section-heading"><h2>消耗分析</h2><button className="btn secondary" onClick={()=>void reload(granularity,windowFilter,days)}>↻ 刷新</button></div>
  <p className="muted">按窗口代差统计已用额度的百分点变化；重置不算消耗，空档表示无样本。</p>
  {loading&&<><div className="skeleton" style={{height:18,width:"30%"}}/><div className="skeleton" style={{height:120,marginTop:12}}/><div className="skeleton" style={{height:120,marginTop:12,width:"70%"}}/></>}
  {!loading&&error&&<p className="error" role="alert">{error} <button className="btn ghost" onClick={()=>void reload(granularity,windowFilter,days)}>重试</button></p>}
  <div className="reminder-filters" role="group" aria-label="粒度">{GRANULARITIES.map(([g,label])=><button key={g} className={"btn "+(granularity===g?"":"secondary")} aria-pressed={granularity===g} onClick={()=>setGranularity(g)}>{label}</button>)}</div>
  {granularity==="daily"&&<div className="reminder-filters" role="group" aria-label="时间范围">{[7,14,30,90].map(d=><button key={d} className={"btn "+(days===d?"":"secondary")} aria-pressed={days===d} onClick={()=>setDays(d)}>{d} 天</button>)}</div>}
  {!loading&&data&&<div className="reminder-filters" role="group" aria-label="窗口">{data.windows.map(w=><button key={w.duration_minutes} className={"btn "+(windowFilter===String(w.duration_minutes)?"":"secondary")} disabled={!w.present} title={w.present?"":"Codex 暂未上报该窗口"} aria-pressed={windowFilter===String(w.duration_minutes)} onClick={()=>setWindowFilter(windowFilter===String(w.duration_minutes)?"":String(w.duration_minutes))}>{DUR_LABEL[w.duration_minutes]||w.duration_minutes+" 分钟"}{w.present?"":"（未上报）"}</button>)}</div>}
  {!loading&&data&&data.coverage&&granularity!=="window"&&<p className="muted compact">总消耗 {total.toFixed(1)} pp · 峰值{unit} {peak.toFixed(1)} pp · 重置 {resets} 次 · 有数据 {data.coverage.days_with_data} 个{unit}{data.coverage.first_day?` · 自 ${data.coverage.first_day} 起积累`:""}</p>}
  {!loading&&granularity==="window"&&data&&data.generations&&data.generations.length>0&&<WindowGenerations gens={data.generations}/>}
  {!loading&&granularity==="window"&&data&&data.generations&&data.generations.length===0&&<p className="muted">最近 7 天内没有足够的窗口样本来绘制当前窗口。</p>}
  {granularity!=="window"&&!loading&&series.length>0&&<BarChart series={series} scale={Math.max(100,total)} granularity={granularity} reference={data&&data.reference_pp!=null?data.reference_pp:0} onTip={setTip} clearTip={()=>setTip(null)}/>}
  {!loading&&granularity!=="window"&&series.length===0&&data&&<p className="muted">所选范围内暂无消耗数据。{data.coverage&&data.coverage.first_day?`自 ${data.coverage.first_day} 起积累。`:""}</p>}
  {granularity==="daily"&&!loading&&series.length>0&&<UsageTable series={series}/>}
  {!loading&&tip&&<div style={{position:"fixed",left:tip.x+14,top:tip.y-12,pointerEvents:"none",background:"var(--panel)",border:"1px solid var(--line)",borderRadius:8,padding:"6px 10px",fontSize:12,zIndex:9,boxShadow:"var(--shadow)"}}>{tip.lines.map((l,i)=><div key={i}>{l}</div>)}</div>}
 </section>;
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
function UsageTable({series}:{series:UsageSeries[]}){
 return <table className="trend-axis" role="table"><thead><tr><th align="left">日期</th><th>消耗 pp</th><th>日终水平</th><th>峰值 pp/时</th><th>重置</th><th>覆盖</th></tr></thead><tbody>{series.slice(-14).reverse().map(s=><tr key={s.bucket_start}><td>{s.bucket_start}</td><td align="center">{s.consumed_pp==null?"—":s.consumed_pp.toFixed(1)}</td><td align="center">{s.level_end==null?"—":s.level_end.toFixed(0)+"%"}</td><td align="center">{s.peak_pp_hour==null?"—":s.peak_pp_hour.toFixed(1)}</td><td align="center">{s.resets??0}</td><td align="center">{s.coverage_hours}h</td></tr>)}</tbody></table>;
}
