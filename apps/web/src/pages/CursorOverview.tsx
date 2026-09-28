import {useEffect,useState} from "react";
import {Link,useSearchParams} from "react-router-dom";
import {Account,api} from "../api";
import Dashboard,{fmtTime,sourceLabel} from "./Dashboard";
import Inbox from "./Inbox";
import Rules from "./Rules";

const tabs=[['overview','额度总览'],['plan','使用计划'],['reminders','提醒中心'],['news','重置雷达'],['settings','提醒设置']];
export default function CursorOverview(){
 const[params]=useSearchParams(),[accounts,setAccounts]=useState<Account[]>([]),[error,setError]=useState('');
 const requested=params.get('tab')||'overview';const tab=tabs.some(([id])=>id===requested)?requested:'overview';
 useEffect(()=>{let active=true;void api.dashboard().then(data=>{if(active)setAccounts(data.accounts.filter(a=>a.provider.slug==='cursor'));}).catch(()=>{if(active)setError('额度暂不可用，已停止新的使用建议。')});return()=>{active=false};},[tab]);
 return <section><p className="eyebrow">CURSOR / PERSONAL COMPANION</p><h1>Cursor 使用助手</h1><p className="lead">按 Cursor 实际额度和重置时间安排工作。采集失败或缺少数据时暂停推断。</p>{error&&<p className="error">{error}</p>}
 <nav className="workspace-tabs" aria-label="Cursor 功能标签">{tabs.map(([id,label])=><Link key={id} to={'/cursor?tab='+id} aria-current={tab===id?'page':undefined}>{label}</Link>)}</nav>
 {tab==='overview'&&<Dashboard/>}
 {tab==='plan'&&<section className="panel"><h2>使用计划</h2>{accounts.length===0?<p>等待 Cursor 额度数据。</p>:accounts.flatMap(a=>a.buckets.map(b=>{const remaining=b.remaining_ratio,reset=b.reset_at?new Date(b.reset_at):null,hours=reset?(reset.getTime()-Date.now())/3600000:null;return <article className="advice-item" key={b.id}><h3>{a.display_name} · {b.scope_key}</h3><p>剩余 {remaining==null?'未知':`${(remaining*100).toFixed(0)}%`} · 重置 {fmtTime(b.reset_at)}</p><p>{remaining!=null&&hours!=null&&hours>0&&b.collection_status!=='stale'?`均匀使用参考：每天最多 ${(remaining/hours*2400).toFixed(1)} 个百分点。`:'数据不足，暂不估算每日预算。'}</p><p className="muted compact">来源：{sourceLabel(b.source_type)} · 采集于 {fmtTime(b.observed_at)}</p></article>}))}</section>}
 {tab==='reminders'&&<Inbox/>}
 {tab==='news'&&<section className="panel"><h2>重置雷达</h2><p>仅显示 Cursor 实际返回的重置和到期时间。</p>{accounts.flatMap(a=>a.buckets.filter(b=>b.reset_at||b.expires_at).map(b=><article className="news-item" key={b.id}><h3>{a.display_name} · {b.scope_key}</h3><p>重置：{fmtTime(b.reset_at)} · 到期：{fmtTime(b.expires_at)}</p><p className="muted compact">最近采集：{fmtTime(b.observed_at)} · {sourceLabel(b.source_type)}</p></article>))}{accounts.every(a=>a.buckets.every(b=>!b.reset_at&&!b.expires_at))&&<p>暂无可核实的重置时间。</p>}</section>}
 {tab==='settings'&&<Rules/>}
 </section>;
}
