import {useEffect,useRef,useState} from 'react';
import {request} from './api';
import {scope} from './session';
declare global {interface Window {aihubDesktop?:{alert:(value:{title:string;body:string;provider?:string})=>Promise<string>;openCodex:()=>Promise<void>;openDashboard?:()=>Promise<void>}}}
import {eligibleStrong,type Alert} from './strongReminderPolicy';
import {CODEX_POLL_MS} from './codexTiming';
import {ActiveProvider,setActiveProvider} from './providerMode';
export default function StrongReminders(){
 const [popup,setPopup]=useState<{title:string;body:string;provider?:string}|null>(null),[error,setError]=useState('');
 const dialog=useRef<HTMLDialogElement>(null),resolve=useRef<((value:string)=>void)|null>(null);
 const key='hub_advisor:'+scope();
 useEffect(()=>{if(popup)dialog.current?.showModal();else dialog.current?.close();},[popup]);
 useEffect(()=>{
  let alive=true,busy=false;const controller=new AbortController();
  async function display(value:{title:string;body:string;provider?:string}){
   if(window.aihubDesktop)return window.aihubDesktop.alert(value);
   return new Promise<string>(done=>{resolve.current=done;setPopup(value);});
  }
  async function pollGeneric(){
   const data=await request<{notifications:{id:string;title:string;body:string;severity:string;status:string;provider_name?:string;provider_slug?:string;provider_account_id?:string}[]}>('/api/v1/notifications',{signal:controller.signal});
   const next=data.notifications.find(n=>n.status==='unread'&&(n.severity==='warning'||n.severity==='critical'));
   if(!next||localStorage.getItem(key+':strong-seen:gen:'+next.id))return false;
   const slug=(next.provider_slug||'') as ActiveProvider|'';
   if(slug==='cursor'||slug==='codex')setActiveProvider(slug);
   const action=await display({title:next.provider_name?`${next.provider_name} · ${next.title}`:next.title,body:next.body,provider:next.provider_name});
   if(!alive||action==='retry')return true;
   if(action==='read')await request('/api/v1/notifications/'+encodeURIComponent(next.id)+'/read',{method:'POST'});
   localStorage.setItem(key+':strong-seen:gen:'+next.id,next.id);setError('');
   return true;
  }
  async function pollCodex(){
   const data=await request<{generated_at:string;quiet:boolean;preferences:{enabled:boolean};alerts:Alert[]}>('/api/v1/codex/overview',{signal:controller.signal});
   if(!alive||!data.preferences.enabled||data.quiet||Date.now()-Date.parse(data.generated_at)>60000)return false;
   const next=[...data.alerts].sort((a,b)=>b.advice.priority-a.advice.priority).find(a=>eligibleStrong(a,Date.now())&&localStorage.getItem(key+':strong-seen:codex:'+a.id)!==a.notified_at);
   if(!next)return false;
   setActiveProvider('codex');
   const action=await display({...next.advice,provider:'Codex'});
   if(!alive||action==='retry')return true;
   if(action==='snooze'||action==='dismiss')await request('/api/v1/codex/alerts/'+encodeURIComponent(next.id),{method:'POST',body:JSON.stringify({action})});
   localStorage.setItem(key+':strong-seen:codex:'+next.id,next.notified_at);setError('');
   return true;
  }
  async function poll(){
   if(busy||localStorage.getItem(key+':strong')==='off')return;
   busy=true;
   try{await navigator.locks.request(key+':strong-lock',{ifAvailable:true},async lock=>{
    if(!lock||!alive)return;
    if(await pollCodex())return;
    await pollGeneric();
   });}catch{if(alive)setError('强提醒暂未连接成功；请检查服务。操作失败时提醒不会标记为已处理。');}finally{busy=false;}
  }
  const test=()=>{if(busy)return;busy=true;void display({title:'强提醒测试 · 额度消耗过快',body:'这是演示弹窗，不代表真实额度异常，也不会改变提醒状态。',provider:'演示'}).finally(()=>{busy=false;});};
  window.addEventListener('aihub-test-alert',test);void poll();const timer=setInterval(()=>void poll(),CODEX_POLL_MS);
  return()=>{alive=false;controller.abort();clearInterval(timer);window.removeEventListener('aihub-test-alert',test);resolve.current?.('retry');};
 },[key]);
 function finish(action:string){setPopup(null);resolve.current?.(action);resolve.current=null;}
 const provider=popup?.provider||'AI Hub';
 return <>{error&&<p className="sync-banner" role="status">{error}</p>}<dialog ref={dialog} className="strong-reminder" aria-labelledby="strong-title" aria-describedby="strong-body" onCancel={e=>{e.preventDefault();finish('read');}}><p className="eyebrow">{provider} · 额度强提醒</p><h2 id="strong-title">{popup?.title}</h2><p id="strong-body">{popup?.body}</p><p className="muted compact">{provider==='Codex'?'当前是账户级额度分析，尚不能定位具体任务的 token 消耗。需要停止时，请在 Codex 中找到对应任务并点击停止。':'采集失败或数据过期时不会用旧数字推断额度耗尽。'}</p><div className="actions"><button className="btn" autoFocus onClick={()=>finish('read')}>已读</button><button className="btn secondary" onClick={()=>finish('snooze')}>1 小时后提醒</button><button className="btn ghost" onClick={()=>finish('dismiss')}>忽略本次</button></div></dialog></>;
}
