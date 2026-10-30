import {useEffect,useRef,useState} from 'react';
import {Clock3,Globe2,RefreshCw,Check} from 'lucide-react';
export type TimeZoneState={id:string;manual:boolean;daylightDisabled:boolean;chinaReady:boolean;laReady:boolean;offsetMinutes:number};
export default function TimeZone(){
 const desktop=window.aihubDesktop;
 const ready=!!desktop?.timeZoneStatus && !!desktop?.timeZoneApply;
 const [state,setState]=useState<TimeZoneState|null>(null),[now,setNow]=useState(Date.now());
 const [busy,setBusy]=useState<'cn'|'la'|null>(null),[loading,setLoading]=useState(false),[error,setError]=useState(''),[statusError,setStatusError]=useState(''),[message,setMessage]=useState('');
 const lock=useRef(false),revision=useRef(0);
 async function refresh(){
  if(lock.current||!ready)return;
  const version=revision.current;setLoading(true);
  try{const value=await desktop!.timeZoneStatus!();if(version===revision.current){setState(value);setStatusError('');}}
  catch(e){if(version===revision.current)setStatusError(e instanceof Error?e.message:'无法读取系统时区。');}
  finally{if(version===revision.current)setLoading(false);}
 }
 useEffect(()=>{
  void refresh();const tick=setInterval(()=>setNow(Date.now()),1000);
  const poll=setInterval(()=>{if(!document.hidden)void refresh();},30000);
  const visible=()=>{if(!document.hidden)void refresh();};window.addEventListener('focus',visible);
  return()=>{revision.current++;clearInterval(tick);clearInterval(poll);window.removeEventListener('focus',visible);};
 },[ready]);
 async function apply(target:'cn'|'la'){
  if(lock.current||!ready)return;
  lock.current=true;revision.current++;setLoading(false);setBusy(target);setError('');setMessage('');
  try{const result=await desktop!.timeZoneApply!(target);setState(result.state);setMessage(result.message);setNow(Date.now());}
  catch(e){setError(e instanceof Error?e.message:'时区切换失败。');}
  finally{lock.current=false;setBusy(null);void refresh();}
 }
 const clock=(zone:string)=>new Intl.DateTimeFormat('zh-CN',{timeZone:zone,hour:'2-digit',minute:'2-digit',second:'2-digit',hour12:false}).format(now);
 const date=(zone:string)=>new Intl.DateTimeFormat('zh-CN',{timeZone:zone,year:'numeric',month:'long',day:'numeric',weekday:'long'}).format(now);
 const laOffset=new Intl.DateTimeFormat('en-US',{timeZone:'America/Los_Angeles',timeZoneName:'shortOffset'}).formatToParts(now).find(p=>p.type==='timeZoneName')?.value;
 const name=state?.id==='China Standard Time'?'中国时间 · 北京 / 上海':state?.id==='Pacific Standard Time'?'洛杉矶时间 · 美国太平洋':state?.id;
 if(!ready)return <section className="panel"><h1>时区切换</h1><p>请使用新版 Windows 桌面端打开本机时区工具。</p></section>;
 return <section className="timezone-workspace">
  <header className="section-heading"><div><p className="eyebrow">LOCAL TOOLS</p><h1>时区切换</h1><p className="muted">洛杉矶与中国，让电脑时间随你切换。</p></div><button className="btn secondary" onClick={()=>{setError('');void refresh();}} disabled={loading||!!busy}><RefreshCw size={16}/>刷新状态</button></header>
  <article className="panel timezone-current"><div><span className="muted"><Clock3 size={16}/> 当前电脑时区</span><h2>{name||'正在读取…'}</h2><p className="muted">{state?(state.manual?'手动时区': '自动时区已开启，切换时将关闭'):'以 Windows 实际设置为准'}</p></div><strong>{state&&Number.isFinite(state.offsetMinutes)?new Date(now+state.offsetMinutes*60000).toISOString().slice(11,19):'--:--:--'}</strong></article>
  {(error||statusError)&&<p className="error" role="alert">{error||statusError}</p>}{message&&<p className="timezone-result" role="status">{message}</p>}
  <div className="timezone-cards">{([{key:'la',name:'洛杉矶',english:'LOS ANGELES',zone:'America/Los_Angeles',active:state?.laReady,offset:`${laOffset?.replace('GMT','UTC')} · ${laOffset==='GMT-7'?'夏令时 PDT':'标准时 PST'}`},{key:'cn',name:'中国',english:'BEIJING / SHANGHAI',zone:'Asia/Shanghai',active:state?.chinaReady,offset:'UTC+08:00 · 北京时间'}] as const).map(city=><article className={'panel timezone-city '+city.key} key={city.key}>
   <div className="section-heading"><div><h2>{city.name}</h2><small className="muted">{city.english}</small></div>{city.active?<span className="timezone-active"><Check size={15}/>当前时区</span>:<Globe2 size={22}/>}</div>
   <time>{clock(city.zone)}</time><p>{date(city.zone)}</p><p className="muted">{city.offset}</p>
   <button className={'btn '+(city.active?'secondary':'')} disabled={!!busy||!state||!!city.active} onClick={()=>void apply(city.key)}>{busy===city.key?'正在切换，请等待系统确认…':city.active?'当前正在使用':`切换到${city.name}时间`}</button>
  </article>)}</div>
  <p className="muted">选择城市将切换整台电脑的系统时区；洛杉矶夏令时自动适配。切换时关闭自动时区，保留自动校时。需要管理员权限时，请在 Windows 系统授权提示中确认。</p>
 </section>;
}
