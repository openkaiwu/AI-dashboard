import { Activity, ArrowRight, RefreshCw, ShieldCheck, Ticket, TrendingDown, TrendingUp } from "lucide-react";
import {useEffect,useRef,useState} from 'react';
import {Link,useSearchParams,useLocation} from 'react-router-dom';
import CodexPlanner from './CodexPlanner';
import {CODEX_POLL_MS,CODEX_STALE_MS} from '../codexTiming';
import UsageTab from '../UsageTab';
import LocalDataSettings from '../LocalDataSettings';
import RadarPanel from '../RadarPanel';
import {showsFiveHour,visibleQuotaWindows} from '../quotaVisibility';
import {request} from '../api';
import {scope} from '../session';
import {remainingLabel} from '../quotaFormat';
import {alertState as reminderState} from '../strongReminderPolicy';
type Pref={enabled:boolean;near_hours:number;spare_percent:number;low_percent:number;pace_lead:number;credit_hours:number;cooldown_hours:number;quiet_start:number;quiet_end:number;utc_offset_minutes:number};
type Advice={kind:string;title:string;body:string;priority:number;interrupt?:boolean;source_name?:string};
type Metric={bucket:string;slot:string;remaining:number;hours_left:number|null;rate_per_hour:number|null;daily_budget:number|null;resets_at:number|null;duration_minutes:number};
type UsagePoint={t:string;used:number};
type Device={id:string;name:string;snapshot:any;history:any[];window_series?:Record<string,UsagePoint[]>;analysis:{state:string;metrics:Metric[];advice:Advice[]}};
type Alert={id:string;advice:Advice;first_seen:string;notified_at:string|null;read_at?:string|null;dismissed:boolean;snoozed_until:string|null};
type News={checked_at:string;status:string;items?:{url:string;published_at:string;approximate_time?:boolean;summary:string}[]};
type Overview={generated_at:string;preferences:Pref;quiet:boolean;devices:Device[];alerts:Alert[];news?:News;plan?:{plan_type:'plus'|'pro'|'prolite'|'unknown';source_type:string}};
const when=(v:string|number|null)=>v==null?'未知':new Date(typeof v==='number'?v*1000:v).toLocaleString();
const hours=(v:number|null)=>v==null?'时间未知':v<=0?'等待重置确认':v<1?`${Math.ceil(v*60)} 分钟`:v>=24?`${(v/24).toFixed(1)} 天`:`${v.toFixed(1)} 小时`;
const labels:Record<string,string>={slow:'建议放慢',use:'适合多用',balanced:'节奏平稳',stale:'等待新数据',unknown:'信息不足'};
const explain:Record<string,string>={slow:'保留后续任务的空间，先完成优先级最高的工作。',use:'把计划中的任务提前，充分使用本轮额度。',balanced:'额度与时间暂未触发提醒，按自己的工作节奏继续。',stale:'电脑可能离线或额度暂不可读，暂停节奏建议。',unknown:'额度窗口信息尚不完整，不推算可用余量。'};
function Trend({d,m}:{d:Device;m:Metric}){
 const key=m.bucket+":"+m.duration_minutes;
 const series=d.window_series&&d.window_series[key];
 const points=series&&series.length>=2?series.map(p=>({x:new Date(p.t).getTime(),y:p.used})):(d.history||[]).flatMap(s=>{const w=s.buckets?.find((x:any)=>x.id===m.bucket)?.[m.slot];return w&&w.resets_at===m.resets_at?[{x:new Date(s.observed_at).getTime(),y:w.used_percent}]:[];});
 if(points.length<2)return <p className="muted compact">正在积累趋势样本，至少 15 分钟后尝试估算速度。</p>;
 const full=series&&series.length>=2;
 const start=points[0].x,end=points[points.length-1].x;
 return <div className="trend"><svg viewBox="0 0 600 110" role="img" aria-label="本轮已用额度趋势，纵轴 0 到 100%"><path d="M10 15H590 M10 55H590 M10 95H590" stroke="currentColor" opacity=".1"/><polyline fill="none" stroke="var(--accent)" strokeWidth="2.5" points={points.map(p=>`${10+(p.x-start)/Math.max(1,end-start)*580},${95-p.y*.8}`).join(' ')}/><text x="12" y="12">100%</text><text x="12" y="108">0%</text></svg><div className="trend-axis"><span>{new Date(start).toLocaleTimeString()}</span><span>已用额度 · {full?'自窗口起点':'最近最多 6 小时'}</span><span>{new Date(end).toLocaleTimeString()}</span></div></div>;
}
export default function CodexOverview(){
 const [params]=useSearchParams();const location=useLocation();
 const tabs=[['overview','额度总览'],['usage','消耗分析'],['plan','使用计划'],['reminders','提醒中心'],['news','重置雷达'],['settings','提醒设置']];
 const requested=params.get('tab')||(location.hash==='#reminders'?'reminders':location.hash==='#rules'?'settings':'overview');
 const tab=tabs.some(t=>t[0]===requested)?requested:'overview';
 const [filter,setFilter]=useState('pending');
 const key='hub_advisor:'+scope();
 const[data,setData]=useState<Overview|null>(()=>{try{return JSON.parse(localStorage.getItem(key)||'null');}catch{return null;}});
 const[error,setError]=useState(''),[selected,setSelected]=useState(''),[message,setMessage]=useState(''),[actionBusy,setActionBusy]=useState('');
 const actionLock=useRef(false);
 const[editing,setEditing]=useState<Pref|null>(null),[saving,setSaving]=useState(false),[clock,setClock]=useState(Date.now());
 async function reload(){try{const next=await request<Overview>('/api/v1/codex/overview');localStorage.setItem(key,JSON.stringify(next));setData(next);setError('');}catch{setError('服务暂不可用，显示上次缓存。已暂停桌面提醒。');}}
 useEffect(()=>{let alive=true,busy=false;const controller=new AbortController();const update=async()=>{if(busy)return;busy=true;try{const next=await request<Overview>('/api/v1/codex/overview',{signal:controller.signal});if(alive){localStorage.setItem(key,JSON.stringify(next));setData(next);setError('');}}catch{if(alive)setError('服务暂不可用，显示上次缓存。已暂停桌面提醒。');}finally{busy=false;}};window.addEventListener('aihub-reminders-changed',update);void update();const timer=setInterval(()=>void update(),CODEX_POLL_MS);const clockTimer=setInterval(()=>setClock(Date.now()),60000);return()=>{alive=false;controller.abort();window.removeEventListener('aihub-reminders-changed',update);clearInterval(timer);clearInterval(clockTimer);};},[key]);

 async function action(id:string,action:string){if(actionLock.current)return;actionLock.current=true;setActionBusy(id);try{await request('/api/v1/codex/alerts/'+encodeURIComponent(id),{method:'POST',body:JSON.stringify({action})});await reload();}catch{setMessage('暂时无法更新提醒，请稍后重试。');}finally{actionLock.current=false;setActionBusy('');}}
 async function save(){if(!editing)return;if(editing.spare_percent<=editing.low_percent){setMessage('多用提醒的剩余阈值必须高于低额度阈值。');return;}setSaving(true);try{await request('/api/v1/codex/preferences',{method:'PATCH',body:JSON.stringify(editing)});setEditing(null);setMessage('规则已保存。电脑端会在下次同步时更新手机。');await reload();}catch(e){setMessage(String(e));}finally{setSaving(false);}}
 async function savePlan(plan_type:string){try{await request('/api/v1/codex/plan',{method:'PUT',body:JSON.stringify({plan_type})});await reload();setMessage('套餐设置已保存；额度和提醒已重新计算。');}catch(e){setMessage(String(e));}}
 const d=data?.devices.find(x=>x.id===selected)||data?.devices[0];const age=clock-Date.parse(d?.snapshot.observed_at||'');const stale=!!error||!d||d.snapshot.status!=='ok'||!Number.isFinite(age)||age < -120000||age>CODEX_STALE_MS;const state=stale?'stale':d.analysis.state;
 const planType=data?.plan?.plan_type||'unknown';
 const metrics=visibleQuotaWindows(d?.analysis.metrics||[],planType);
 const credits=d?.snapshot.reset_credits,news=d?.snapshot.news ?? data?.news,p=editing||data?.preferences;
 const pending=(data?.alerts||[]).filter(a=>reminderState(a,clock)==='pending');
 const alertState=(a:Alert)=>reminderState(a,clock);
 const visible=(data?.alerts||[]).filter(a=>alertState(a)===filter);
 const top=stale?undefined:[...(d?.analysis.advice||[])].filter(a=>a.kind==="slow"||a.kind==="use").sort((a,b)=>b.priority-a.priority)[0];
 const fields:{key:keyof Pref;label:string;min:number;max:number;hint:string}[]=[
 {key:'near_hours',label:'临近重置 · 小时',min:1,max:168,hint:'同时不超过周期的最后 20%'},{key:'spare_percent',label:'多用提醒 · 剩余至少 %',min:10,max:95,hint:'须高于低额度阈值；其他窗口紧张时不催促多用'},
 {key:'low_percent',label:'低额度提醒 · 剩余不超过 %',min:1,max:30,hint:'优先于多用建议'},{key:'pace_lead',label:'超前消耗 · 百分点',min:5,max:70,hint:'已用比例超出时间进度的容许差'},
 {key:'credit_hours',label:'重置卡到期前 · 小时',min:1,max:168,hint:'仅对已知到期且仍可用的卡提醒'},
 {key:'quiet_start',label:'免打扰开始 · 当地时',min:0,max:23,hint:'默认晚上 23 点'},{key:'quiet_end',label:'免打扰结束 · 当地时',min:0,max:23,hint:'默认早上 8 点；起止相同为关闭'},
 {key:'utc_offset_minutes',label:'时区 · UTC 偏移分钟',min:-720,max:840,hint:'北京时间 480；出差时可调整'}];
 if(!data&&!error)return <section className="panel"><p className="eyebrow">CODEX WORKSPACE</p><div className="skeleton" style={{height:26,width:"38%"}}/><div className="skeleton" style={{height:64,marginTop:16}}/><div className="advisor-metrics" style={{marginTop:14}}><div className="skeleton" style={{height:150,width:210}}/><div className="skeleton" style={{height:150,width:210}}/></div></section>;
 return <section className="advisor"><header className="advisor-header"><div><p className="eyebrow">CODEX WORKSPACE</p><h1>{({overview:'额度总览',usage:'消耗分析',plan:'使用计划',reminders:'提醒中心',news:'重置消息',settings:'提醒设置'} as Record<string,string>)[tab]}</h1><p className="lead">{({overview:'掌握剩余额度，从容安排下一步。',usage:'查看额度变化与消耗趋势，了解你的使用节奏。',plan:'预留、分配、复盘，让下一次重置之前的工作更从容。',reminders:'集中处理额度、节奏和到期提醒，已提醒事件不会再次弹出。',news:'查看 Tibo 原帖和最近检查结果，账户变化以实际采集为准。',settings:'统一管理规则阈值、免打扰时间和此设备的通知权限。'} as Record<string,string>)[tab]}</p></div><div className="advisor-tools">{data&&data.devices.length>1&&<select aria-label="分析哪台电脑" value={d?.id} onChange={e=>setSelected(e.target.value)}>{data.devices.map(x=><option key={x.id} value={x.id}>{x.name}</option>)}</select>}{tab!=='usage'&&<button className="btn secondary" onClick={()=>void reload()}><RefreshCw size={15} aria-hidden="true"/>刷新</button>}</div></header>
 <div className="advisor-status"><span className={'status-dot'+(stale?' offline':'')}/><span>{stale?'等待电脑新数据':d?.name+(window.aihubDesktop?' · 已保存':' · 已同步')}</span><span>{d?.snapshot?.observed_at?when(d.snapshot.observed_at):'尚未采集'}</span>{data&&data.plan&&<span className="device-tag">{data.plan.plan_type==='unknown'?'套餐未确认':data.plan.plan_type==='plus'?'Plus':data.plan.plan_type==='pro'?'Pro':'Pro Lite'}</span>}<Link to="/connections">管理连接 ↗</Link></div>
 {error&&<p className="error" role="status">{error}</p>}{message&&<p className="sync-banner" role="status">{message}<button className="btn ghost" onClick={()=>setMessage('')}>关闭</button></p>}
 {data?.plan?.plan_type==='unknown'&&<p className="sync-banner">Codex 套餐尚未确认。<Link to="/codex?tab=settings">前往设置，确认套餐</Link></p>}

 <nav className="workspace-tabs" aria-label="Codex 功能标签">{tabs.map(([id,label])=><Link key={id} to={'/codex?tab='+id} aria-current={tab===id?'page':undefined}>{label}{id==='reminders'&&pending.length>0&&<span>{pending.length}</span>}</Link>)}</nav>
 {tab==='plan'&&<CodexPlanner planType={planType} device={d} stale={stale||state==='unknown'||state==='stale'}/>}
 {tab==='overview'&&(!data?.devices.length?<div className="empty-notes"><h2>先连接电脑上的 Codex</h2><p>真实额度到达后，自动生成节奏建议和到期提醒。</p><Link className="btn" to="/connections">连接电脑 ↗</Link></div>:<>
 <div className={'advice-hero '+state}><div><span className="advisor-kicker"><ShieldCheck size={14} aria-hidden="true"/>现在的建议 · {labels[state]}</span><h2>{top?.title||labels[state]}</h2><p>{top?.body||explain[state]}</p><Link to="/codex?tab=reminders">查看提醒<ArrowRight size={14} aria-hidden="true"/></Link></div><div className="hero-symbol" aria-hidden="true">{state==='slow'?<TrendingDown size={34}/>:state==='use'?<TrendingUp size={34}/>:<Activity size={34}/>}</div></div>
 <div className="overview-section-head"><h2>我的额度</h2><span>以最近一次采集为准</span></div>
 <div className="advisor-metrics">{metrics.map((m,i)=><article className="quota-tile" key={m.bucket+m.slot}><div className="tile-caption"><span>{m.duration_minutes%1440===0?m.duration_minutes/1440+' 天':m.duration_minutes/60+' 小时'}额度</span><span>{m.bucket}</span></div><div className="quota-number">{remainingLabel(m.remaining)}<span>% 剩余</span></div><progress value={m.remaining} max={100} aria-label={m.bucket+'剩余额度'}/><div className="tile-row"><span>距离重置</span><strong>{hours(m.hours_left)}</strong></div><div className="tile-row"><span>均匀使用预算</span><strong>{m.daily_budget==null?'暂不可估':m.daily_budget.toFixed(1)+'% / 天'}</strong></div><p className="muted compact">{m.rate_per_hour==null?'样本不足，暂不预测耗尽时间':`近一小时消耗 ${m.rate_per_hour.toFixed(1)} 个百分点 / 小时`}</p>{i===0&&d&&<Trend d={d} m={m}/>}</article>)}
 {(showsFiveHour(planType)&&!metrics.some(m=>m.duration_minutes===300))&&<p className="muted compact">五小时额度：Codex 暂未返回该窗口（以 Codex 实际显示为准）。</p>}
<article className="quota-tile credit-tile"><div className="tile-caption"><span>备用重置卡</span><Ticket size={17} aria-hidden="true"/></div><div className="quota-number">{credits==null?'—':credits.available_count}<span>{credits==null?'状态未知':'张可用'}</span></div><p className="muted">{credits==null?'Codex 尚未返回重置卡信息。':credits.available_count===0?'当前没有可用的重置卡。':'接近到期时会单独提醒。只提醒，不自动兑换。'}</p>{credits?.items?.map((c:any)=><div className="credit-row" key={c.id}><span>重置卡</span><strong>{c.expires_at?when(c.expires_at)+' 到期':'到期时间未知'}</strong>{c.expires_at&&<span className="muted">（{Math.max(0,Math.ceil((c.expires_at*1000-Date.now())/864e5))} 天后）</span>}</div>)}{credits&&credits.available_count>credits.items.length&&<p className="muted compact">还有 {credits.available_count-credits.items.length} 张未返回到期明细。</p>}<p className="muted compact">是否满足兑换条件，以 Codex 中的实际提示为准。</p></article></div></>)}
 {(tab==='usage'||tab==='settings')&&<LocalDataSettings/>}
 {tab==='usage'&&<UsageTab key={planType} planType={planType}/>}
 {tab==='reminders'&&<section className="panel" id="reminders"><div className="section-heading"><h2>值得留意</h2><span className="device-tag">{pending.length} 条提醒</span></div><p className="muted compact">{data?.quiet?'当前免打扰，站内仍展示，桌面不弹出。':'同一事件只在主手机提醒一次；恢复列表状态不会再次弹出。'}</p>{!data?.preferences.enabled&&<p className="muted">提醒已暂停，分析仍更新。</p>}<div className="reminder-filters" aria-label="提醒状态">{[['pending','待处理'],['read','已读'],['snoozed','已暂缓'],['dismissed','已忽略']].map(([id,label])=><button key={id} className={'btn '+(filter===id?'':'secondary')} aria-pressed={filter===id} onClick={()=>setFilter(id)}>{label} · {(data?.alerts||[]).filter(a=>alertState(a)===id).length}</button>)}</div><p className="muted compact">显示仍符合触发条件的事件；已失效事件会自动移除。</p>{visible.length===0?<div className="calm-empty"><span>✓</span><h3>此分类暂无提醒</h3><p>临近重置、消耗过快、重置卡到期时会出现在这里。</p></div>:visible.map(a=><article className={'advice-item '+a.advice.kind} key={a.id}><div className="tile-caption"><span>{({slow:'节奏提醒',use:'额度利用',credit:'到期提醒',news:'外部消息'} as Record<string,string>)[a.advice.kind]}</span><time>{when(a.first_seen)}</time></div><h3>{a.advice.title}</h3><p className="muted compact">{a.advice.source_name?`来源：${a.advice.source_name} · `:""}{a.advice.interrupt===false?"页面建议":"重要提醒"}</p><p>{a.advice.body}</p><div className="actions">{filter!=='pending'?<button className="btn secondary" disabled={!!actionBusy} onClick={()=>void action(a.id,'restore')}>恢复到待处理</button>:<><button className="btn" disabled={!!actionBusy} onClick={()=>void action(a.id,'read')}>已读</button><button className="btn ghost" disabled={!!actionBusy} onClick={()=>void action(a.id,'dismiss')}>忽略本次</button></>}{filter==='snoozed'&&<span className="muted compact">{when(a.snoozed_until)} 恢复</span>}</div></article>)}</section>}
 {tab==='news'&&<RadarPanel news={news} metrics={metrics} history={d?.history||[]} observedAt={d?.snapshot.observed_at} stale={stale} now={clock}/>}
 {tab==='settings'&&p&&<section className="panel rules-panel" id="rules"><div className="section-heading"><div><h2>按你的习惯提醒</h2><p className="muted">规则跨端保存，通知权限仅在此设备生效。</p></div><span className="muted">桌面静默 · 手机主动提醒</span></div><label className="field plan-setting"><span>Codex 套餐 · {data?.plan?.source_type==='manual'?'手动设置':'采集识别'}</span><select value={data?.plan?.plan_type||'unknown'} onChange={e=>void savePlan(e.target.value)}><option value="unknown">未知</option><option value="plus">Plus · 显示五小时额度</option><option value="pro">Pro · 按周窗口显示</option><option value="prolite">Pro Lite · 按周窗口显示</option></select></label><div className="strong-settings"><p>电脑与网页保持静默，提醒由主提醒手机发出。同一事件只提醒一次；手机至少间隔 30 分钟合并提醒。快速消耗需在 15 分钟内实际超过 5 个百分点。</p></div><label className="toggle-row"><input type="checkbox" checked={p.enabled} onChange={e=>setEditing({...p,enabled:e.target.checked})}/>启用额度与到期提醒</label><details open><summary>调整规则阈值与免打扰时间</summary><div className="rule-fields">{fields.map(f=><label className="field" key={f.key}><span>{f.label}</span><input type="number" min={f.min} max={f.max} value={p[f.key] as number} onChange={e=>setEditing({...p,[f.key]:Number(e.target.value)})}/><small className="muted">{f.hint}</small></label>)}</div></details>{editing&&<div className="actions"><button className="btn" disabled={saving} onClick={()=>void save()}>保存规则</button><button className="btn ghost" onClick={()=>setEditing(null)}>取消</button></div>}<p className="muted compact">电脑需开启同步到手机，手机需启用后台同步并允许额度通知。手机不可达时电脑保持静默。</p></section>}
 </section>;
}
