import {useCallback,useEffect,useRef,useState} from 'react';
import {Link} from 'react-router-dom';
import {Activity,ArrowUpRight,ChartNoAxesColumn,Clock3,CloudOff,Info,RefreshCw} from 'lucide-react';
import {request} from './api';
import {showsFiveHour} from './quotaVisibility';
import {dailyPoints,emptyResources,hourlyPoints,latestFive,sampleTotal,settleResource} from './consumption';
import type {ChartPoint,UsageData,UsageResources} from './consumption';

type Range='24h'|'7d'|'30d';
const ranges:Range[]=['24h','7d','30d'];
const labels:Record<Range,string>={'24h':'近 24 小时','7d':'近 7 天','30d':'近 30 天'};
const when=(date:string)=>new Date(date).toLocaleString();

export default function UsageTab({planType}:{planType:string}) {
  const [basis,setBasis]=useState('10080');
  const [range,setRange]=useState<Range>('7d');
  const [revision,setRevision]=useState(0);
  const scope=planType+':'+basis;
  const [result,setResult]=useState({scope,resources:emptyResources()});
  const current=result.scope===scope?result.resources:emptyResources();
  const retry=useCallback(()=>setRevision(v=>v+1),[]);
  const scopeRef=useRef(scope);scopeRef.current=scope;
  useEffect(()=>{
    const controller=new AbortController();let active=true;
    const keys:('hourly'|'daily'|'five')[]=showsFiveHour(planType)?['hourly','daily','five']:['hourly','daily'];
    setResult(previous=>({scope,resources:Object.fromEntries(Object.entries(previous.scope===scope?previous.resources:emptyResources()).map(([key,resource])=>[key,{...resource,loading:keys.includes(key as typeof keys[number])}])) as UsageResources}));
    for(const key of keys) {
      const query=key==='five'?'granularity=window&window=300':key==='hourly'?`granularity=hourly&hours=24&window=${basis}`:`granularity=daily&days=30&window=${basis}`;
      void request<UsageData>('/api/v1/codex/consumption?'+query,{signal:AbortSignal.any([controller.signal,AbortSignal.timeout(15000)])}).then(data=>{
        const rows=key==='five'?data.generations:key==='hourly'?data.hourly:data.series;
        if(!Array.isArray(rows))throw new Error('Missing consumption series');
        return {...data,generated_at:data.generated_at||new Date().toISOString()};
      }).catch(()=>undefined).then(data=>{
        if(!active||scopeRef.current!==scope)return;
        setResult(previous=>({scope,resources:{...previous.resources,[key]:settleResource(previous.resources[key],data)}}));
      });
    }
    return()=>{active=false;controller.abort();};
  },[scope,basis,planType,revision]);
  return <ConsumptionView planType={planType} basis={basis} onBasis={setBasis} range={range} onRange={setRange} resources={current} onRefresh={retry}/>;
}

export function ConsumptionView({planType,basis,onBasis,range,onRange,resources,onRefresh,now=Date.now()}:{planType:string;basis:string;onBasis:(basis:string)=>void;range:Range;onRange:(range:Range)=>void;resources:UsageResources;onRefresh:()=>void;now?:number}) {
  const five=showsFiveHour(planType),entries=five?Object.values(resources):[resources.hourly,resources.daily];
  const loading=entries.some(r=>r.loading),allFailed=entries.every(r=>r.error&&!r.data)&&!loading;
  const points:Record<Range,ChartPoint[]>={'24h':hourlyPoints(resources.hourly.data,now),'7d':dailyPoints(resources.daily.data,7,now),'30d':dailyPoints(resources.daily.data,30,now)};
  const resource=(r:Range)=>r==='24h'?resources.hourly:resources.daily;
  const selected=resource(range),series=points[range],total=sampleTotal(series),known=series.filter(p=>p.value!==null);
  const currentFive=latestFive(resources.five.data);
  const fiveExpired=!!currentFive?.resetsAt&&currentFive.resetsAt*1000<=now;
  const basisLabel=basis==='300'?'5 小时额度':'周额度';
  return <section className="consumption-workspace" aria-label="消耗分析详情" aria-busy={loading}>
    <div className="consumption-toolbar"><div className="consumption-basis"><ChartNoAxesColumn size={18} aria-hidden="true"/>{five?<label>统计额度<select aria-label="统计额度" value={basis} onChange={e=>onBasis(e.target.value)}><option value="10080">周额度</option><option value="300">5 小时额度</option></select></label>:<strong>周额度消耗</strong>}<span className="muted compact">重置不计入消耗</span></div><button className="btn secondary" disabled={loading} onClick={onRefresh}><RefreshCw size={15} className={loading?'is-spinning':''} aria-hidden="true"/>{loading?'正在更新':'刷新数据'}</button></div>
    {planType==='unknown'&&<p className="consumption-notice"><Info size={16} aria-hidden="true"/>套餐尚未确认，暂按周额度展示。<Link to="/codex?tab=settings">前往提醒设置</Link></p>}
    {allFailed?<UsagePlaceholder kind="error" title="暂时无法获取消耗数据" onRetry={onRefresh}/>:<>
      <div className={'consumption-summary'+(five?' has-five':'')}>
        {five&&<article className="consumption-stat five-stat"><span className="stat-label"><Clock3 size={16} aria-hidden="true"/>5 小时窗口<span className="device-tag">Plus</span></span>{resources.five.loading&&!resources.five.data?<div className="skeleton stat-skeleton"/>:<strong>{currentFive&&!fiveExpired?currentFive.remaining.toFixed(0):'—'}{currentFive&&!fiveExpired&&<small>% 剩余</small>}</strong>}<span className="stat-caption">{resources.five.error?(resources.five.data?'更新失败 · 上次数据':'获取失败'):fiveExpired?'已到重置时间，等待新采集':currentFive?'最近采集 '+new Date(currentFive.observed).toLocaleTimeString():resources.five.loading?'正在读取额度':'暂无窗口样本'}</span></article>}
        {ranges.map(r=>{const res=resource(r),value=sampleTotal(points[r]);return <button key={r} className={'consumption-stat'+(r===range?' selected':'')} onClick={()=>onRange(r)} aria-pressed={r===range}><span className="stat-label">{labels[r]}<ArrowUpRight size={16} aria-hidden="true"/></span>{res.loading&&!res.data?<div className="skeleton stat-skeleton"/>:<strong>{value===null?'—':value.toFixed(1)}{value!==null&&<small>个百分点</small>}</strong>}<span className="stat-caption">{res.error?(res.data?'更新失败 · 上次数据':'获取失败'):res.loading?'正在更新':value===null?'等待采集样本':`${basisLabel} · ${points[r].filter(p=>p.value!==null).length}/${points[r].length} ${r==='24h'?'小时':'天'}有样本`}</span></button>;})}
      </div>
      {entries.some(r=>r.error)&&<p className="consumption-notice" role="status"><CloudOff size={16} aria-hidden="true"/>部分数据更新失败；已有数据保留，未获取的数据以“—”显示。</p>}
      <section className="panel consumption-chart-panel">
        <div className="consumption-chart-header"><div><p className="eyebrow">USAGE TREND</p><h2>消耗趋势</h2><p className="muted compact">{basisLabel} · {range==='24h'?'逐小时记录':'按账户时区统计，含今天'}</p></div><div className="consumption-ranges" role="group" aria-label="图表时间范围">{ranges.map(r=><button key={r} className={r===range?'active':''} aria-pressed={r===range} onClick={()=>onRange(r)}>{labels[r]}</button>)}</div></div>
        {selected.loading&&!selected.data?<div className="consumption-loading" role="status"><div className="skeleton"/><span>正在读取消耗记录…</span></div>:!selected.data&&selected.error?<UsagePlaceholder kind="error" title="这个时间范围暂时无法加载" onRetry={onRefresh}/>:total===null?<UsagePlaceholder kind="empty" title="这段时间还没有消耗样本" onRetry={onRefresh}/>:<>
          <div className="consumption-chart-meta"><span><i className="legend-sample"/>已记录的消耗</span><span><i className="legend-gap"/>无样本</span><span>单位：百分点（pp）</span></div>
          <UsageChart key={range+':'+basis} points={series}/>
          <div className="consumption-insights"><div><span>已记录消耗</span><strong>{total.toFixed(1)}<small> pp</small></strong></div><div><span>{range==='24h'?'有样本小时均值':'有样本日均'}</span><strong>{(total/known.length).toFixed(1)}<small> pp</small></strong></div><div><span>样本覆盖</span><strong>{known.length}<small> / {series.length} {range==='24h'?'小时':'天'}</small></strong></div></div>
          {known.length<series.length&&<p className="muted compact">存在采集空档，合计只包含已记录部分；空档不代表零消耗。</p>}
        </>}
        {selected.data?.generated_at&&<p className={'consumption-updated'+(selected.error?' stale':'')}>{selected.error?'刷新失败，显示上次成功获取的数据':'数据更新于'} · {when(selected.data.generated_at)}</p>}
      </section>
    </>}
    <p className="consumption-footnote"><Info size={15} aria-hidden="true"/><span>1 个百分点表示本窗口完整额度的 1%。不同额度窗口分别统计；重置不算消耗，真实零消耗才显示 0。</span></p>
  </section>;
}

function UsagePlaceholder({kind,title,onRetry}:{kind:'error'|'empty';title:string;onRetry:()=>void}) {
  const Icon=kind==='error'?CloudOff:Activity;
  return <div className={'consumption-placeholder '+kind} role={kind==='error'?'alert':'status'}><span className="placeholder-icon"><Icon size={26} aria-hidden="true"/></span><h3>{title}</h3><p>{kind==='error'?'请检查服务器连接后重试。当前无法判断消耗量。':'电脑持续在线并完成采集后，这里会显示趋势。'}</p><div className="actions"><button className="btn" onClick={onRetry}><RefreshCw size={15} aria-hidden="true"/>{kind==='error'?'重新加载':'刷新数据'}</button><Link className="btn secondary" to="/connections">检查电脑连接</Link></div></div>;
}

function UsageChart({points}:{points:ChartPoint[]}) {
  const [index,setIndex]=useState<number|null>(null);
  const max=Math.max(1,...points.map(p=>p.value||0));
  const scale=Math.ceil(max*1.15*10)/10;
  const chosen=points[index??points.length-1];
  return <div className="consumption-chart"><div className="consumption-plot"><div className="consumption-axis"><span>{scale.toFixed(1)}</span><span>{(scale/2).toFixed(1)}</span><span>0</span></div><div className="consumption-bars" style={{gridTemplateColumns:`repeat(${points.length},minmax(0,1fr))`}}>{points.map((p,i)=><button key={p.key} className={'consumption-bar'+(p.value===null?' missing':'')+(i===index?' highlighted':'')} onMouseEnter={()=>setIndex(i)} onFocus={()=>setIndex(i)} onClick={()=>setIndex(i)} aria-label={`${p.key}，${p.value===null?'无采集样本':`消耗 ${p.value.toFixed(1)} 个百分点，重置 ${p.resets} 次`}`}><span style={{height:p.value===null?'3px':`${Math.max(1,p.value/scale*100)}%`}}/>{p.resets>0&&<i aria-hidden="true">·</i>}</button>)}</div></div><div className="consumption-x-axis"><span>{points[0].label}</span><span>{points[Math.floor(points.length/2)].label}</span><span>{points.at(-1)!.label}</span></div><p className="consumption-selection" aria-live="polite"><strong>{chosen.label}</strong><span>{chosen.value===null?'暂无采集样本':`消耗 ${chosen.value.toFixed(1)} pp`}</span>{chosen.resets>0&&<span>重置 {chosen.resets} 次</span>}<small>点选柱条查看明细</small></p></div>;
}
