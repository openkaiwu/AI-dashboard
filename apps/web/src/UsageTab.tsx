import {useEffect,useState} from "react";
import {request} from "./api";
type UsageSeries={bucket_start:string;consumed_pp:number|null;level_end:number|null;peak_pp_hour:number|null;resets:number|null;coverage_hours:number};
type UsageGeneration={resets_at:number|null;start:string;end:string;points:{t:string;used:number}[]};
type UsageWindow={id:string;duration_minutes:number;present:boolean};
type UsageData={generated_at:string;tz_offset_minutes:number;windows:UsageWindow[];series?:UsageSeries[];generations?:UsageGeneration[];coverage?:{first_day:string;days_with_data:number}};
const GRANULARITIES:[string,string][]=[["window","当前窗口"],["daily","每日"],["weekly","每周"],["monthly","每月"]];
const DUR_LABEL:Record<number,string>={300:"5 小时窗口",10080:"7 天窗口"};
const when=(v:string)=>new Date(v).toLocaleString();
export default function UsageTab(){
 const[granularity,setGranularity]=useState("daily");
 const[windowFilter,setWindowFilter]=useState("");
 const[data,setData]=useState<UsageData|null>(null),[error,setError]=useState("");
 useEffect(()=>{void reload(granularity,windowFilter);},[granularity,windowFilter]);
 async function reload(g:string,w:string){try{setData(await request<UsageData>(`/api/v1/codex/consumption?granularity=${g}${w?`&window=${w}`:""}`));setError("");}catch(e){setError(String(e));}}
 const series=(data?.series||[]).filter(s=>s.consumed_pp!=null);
 const total=series.reduce((a,s)=>a+(s.consumed_pp||0),0);
 const peak=series.reduce((a,s)=>Math.max(a,s.consumed_pp||0),0);
 const resets=series.reduce((a,s)=>a+(s.resets||0),0);
 const unit=granularity==="daily"?"日":granularity==="weekly"?"周":"月";
 const scale=Math.max(100,total);
 return <section className="panel">
  <div className="section-heading"><h2>消耗分析</h2><button className="btn secondary" onClick={()=>void reload(granularity,windowFilter)}>↻ 刷新</button></div>
  <p className="muted">按窗口代差统计已用额度的百分点变化；重置不算消耗，空档表示无样本。</p>
  {error&&<p className="error" role="alert">{error}</p>}
  <div className="reminder-filters" role="group" aria-label="粒度">{GRANULARITIES.map(([g,label])=><button key={g} className={"btn "+(granularity===g?"":"secondary")} aria-pressed={granularity===g} onClick={()=>setGranularity(g)}>{label}</button>)}</div>
  {data&&<div className="reminder-filters" role="group" aria-label="窗口">{data.windows.map(w=><button key={w.duration_minutes} className={"btn "+(windowFilter===String(w.duration_minutes)?"":"secondary")} disabled={!w.present} title={w.present?"":"Codex 暂未上报该窗口"} aria-pressed={windowFilter===String(w.duration_minutes)} onClick={()=>setWindowFilter(windowFilter===String(w.duration_minutes)?"":String(w.duration_minutes))}>{DUR_LABEL[w.duration_minutes]||w.duration_minutes+" 分钟"}{w.present?"":"（未上报）"}</button>)}</div>}
  {data&&data.coverage&&granularity!=="window"&&<p className="muted compact">总消耗 {total.toFixed(1)} pp · 峰值{unit} {peak.toFixed(1)} pp · 重置 {resets} 次 · 有数据 {data.coverage.days_with_data} 个{unit}{data.coverage.first_day?` · 自 ${data.coverage.first_day} 起积累`:""}</p>}
  {granularity==="window"&&data&&data.generations&&data.generations.length>0&&<WindowGenerations gens={data.generations}/>}
  {granularity==="window"&&data&&data.generations&&data.generations.length===0&&<p className="muted">最近 7 天内没有足够的窗口样本来绘制当前窗口。</p>}
  {granularity!=="window"&&series.length>0&&<BarChart series={series} scale={scale} granularity={granularity}/>}
  {granularity!=="window"&&series.length===0&&data&&<p className="muted">所选范围内暂无消耗数据。{data.coverage?.first_day?`自 ${data.coverage.first_day} 起积累。`:""}</p>}
  {granularity==="daily"&&series.length>0&&<table className="trend-axis" role="table"><thead><tr><th align="left">日期</th><th>消耗 pp</th><th>日终水平</th><th>峰值 pp/时</th><th>重置</th><th>覆盖</th></tr></thead><tbody>{series.slice(-14).reverse().map(s=><tr key={s.bucket_start}><td>{s.bucket_start}</td><td align="center">{s.consumed_pp==null?"—":s.consumed_pp.toFixed(1)}</td><td align="center">{s.level_end==null?"—":s.level_end.toFixed(0)+"%"}</td><td align="center">{s.peak_pp_hour==null?"—":s.peak_pp_hour.toFixed(1)}</td><td align="center">{s.resets??0}</td><td align="center">{s.coverage_hours}h</td></tr>)}</tbody></table>}
 </section>;
}
function WindowGenerations({gens}:{gens:UsageGeneration[]}){
 const all=gens.flatMap(g=>g.points);
 const start=Math.min(...all.map(p=>new Date(p.t).getTime())),end=Math.max(...all.map(p=>new Date(p.t).getTime()));
 const x=(t:string)=>10+(new Date(t).getTime()-start)/Math.max(1,end-start)*580;
 const y=(u:number)=>95-u*.8;
 return <div className="trend"><svg viewBox="0 0 600 110" role="img" aria-label="当前窗口已用额度趋势，纵轴 0 到 100%"><path d="M10 15H590 M10 55H590 M10 95H590" stroke="currentColor" opacity=".1"/>{gens.map((g,gi)=>g.points.length>=2?<polyline key={gi} fill="none" stroke="var(--accent)" strokeWidth="2.5" points={g.points.map(p=>`${x(p.t)},${y(p.used)}`).join(" ")}/>:<circle key={gi} cx={x(g.points[0].t)} cy={y(g.points[0].used)} r="3" fill="var(--accent)"/>)}<text x="12" y="12">100%</text><text x="12" y="108">0%</text></svg><div className="trend-axis"><span>{when(gens[0].start)}</span><span>已用额度 · 当前窗口（重置处分段）</span><span>{when(gens[gens.length-1].end)}</span></div></div>;
}
function BarChart({series,scale,granularity}:{series:UsageSeries[];scale:number;granularity:string}){
 const bars=series.filter(s=>s.consumed_pp!=null);
 const bw=Math.max(2,580/Math.max(1,series.length)-4);
 return <div className="trend"><svg viewBox="0 0 600 140" role="img" aria-label={`每个${granularity==="daily"?"日":granularity==="weekly"?"周":"月"}的额度消耗，纵轴按 ${scale.toFixed(0)} pp 缩放`}>
  <path d="M10 20H590 M10 70H590 M10 120H590" stroke="currentColor" opacity=".1"/>
  {series.map((s,i)=>{const cx=10+i*(580/series.length)+bw/2;
   return s.consumed_pp==null?null:<g key={s.bucket_start}><title>{`${s.bucket_start}：消耗 ${s.consumed_pp.toFixed(1)} pp${s.resets?` · 重置 ${s.resets} 次`:""}${s.coverage_hours?` · 覆盖 ${s.coverage_hours} 小时`:"（无样本）"}`}</title><rect x={cx-bw/2} y={120-(s.consumed_pp||0)/scale*100} width={bw} height={Math.max(1,(s.consumed_pp||0)/scale*100)} fill="var(--accent)" opacity={s.coverage_hours===0?0.3:0.85}/>{s.resets?s.resets>0&&<text x={cx} y={114-(s.consumed_pp||0)/scale*100} textAnchor="middle" fontSize="10">▲</text>:null}</g>;})}
  <text x="12" y="14">{scale.toFixed(0)} pp</text>
 </svg>
 <div className="trend-axis"><span>{bars[0]?.bucket_start||""}</span><span>消耗（百分点）· ▲ 重置</span><span>{bars[bars.length-1]?.bucket_start||""}</span></div></div>;
}
