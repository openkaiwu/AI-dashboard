// Local development entry only. Vite's production build includes index.html only.
// All requests are handled here; this preview never connects to a real account.
import {useState} from 'react';
import { createRoot } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import './styles.css';
import './foundation.css';
import './interface.css';

if (!import.meta.env.DEV) throw new Error('Preview is available in development only');
const previewParams=new URLSearchParams(location.search);
let previewUsageMode=previewParams.get('usage_state')||'ready';
const now = Date.now();
const iso = (offset = 0) => new Date(now + offset).toISOString();
const prefs = {enabled:true,near_hours:12,spare_percent:50,low_percent:15,pace_lead:20,credit_hours:48,cooldown_hours:4,quiet_start:23,quiet_end:8,utc_offset_minutes:480};
const metrics = [
  {bucket:'codex',slot:'primary',remaining:72,hours_left:3.5,rate_per_hour:5.2,daily_budget:49.4,resets_at:Math.floor(now/1000)+12600,duration_minutes:300},
  {bucket:'codex',slot:'secondary',remaining:86,hours_left:124,rate_per_hour:1.1,daily_budget:16.6,resets_at:Math.floor(now/1000)+446400,duration_minutes:10080},
];
const overview = {
  generated_at:iso(),preferences:prefs,quiet:true,plan:{plan_type:previewParams.get('plan')||'pro',source_type:'manual'},
  alerts:[],news:{status:'checked',checked_at:iso(),items:[]},
  devices:[{id:'preview-desktop',name:'我的工作电脑',snapshot:{status:'ok',observed_at:iso(),reset_credits:{available_count:2,items:[{id:'card-1',expires_at:Math.floor(now/1000)+86400*12},{id:'card-2',expires_at:Math.floor(now/1000)+86400*26}]}},history:[],
    window_series:{'codex:300':Array.from({length:12},(_,i)=>({t:iso((i-11)*900000),used:5+i*2.1}))},
    analysis:{state:'balanced',metrics,advice:[]}}],
};
const session = {token:'local-preview-only',refresh_token:'local-preview-only',device_id:'preview-device',user:{id:'preview-user',email:'preview@example.com',role:'user'}};
// The desktop session adapter keeps preview credentials in memory, out of real storage.
const previewStorage={auto_sync:false,directory:'C:\\Users\\Example\\AppData\\Roaming\\aihub-desktop\\local-data\\preview',observed_at:iso()};
window.aihubDesktop = {sessionGet:()=>session,sessionSet:()=>true,localStatus:async(_id:string,patch?:{auto_sync:boolean})=>Object.assign(previewStorage,patch||{}),syncLocalData:async()=>({...previewStorage,last_sync:iso()}),openCodex:async()=>{throw new Error('界面预览不会启动应用');}} as unknown as NonNullable<typeof window.aihubDesktop>;
class PreviewChannel extends EventTarget { close() {} }
window.WebSocket = PreviewChannel as unknown as typeof WebSocket;
window.EventSource = PreviewChannel as unknown as typeof EventSource;
window.fetch = async (input, init) => {
  const url = new URL(typeof input==='string'?input:input instanceof URL?input.href:input.url, location.origin);
  const path = url.pathname;
  let data: unknown;
  if(path==='/api/v1/codex/overview') {
    overview.generated_at = new Date().toISOString();
    overview.devices[0].snapshot.observed_at = overview.generated_at;
    data=overview;
  }
  else if(path==='/api/v1/codex/radar') {data={news:overview.news};}
  else if(path==='/api/v1/codex/preferences') {Object.assign(prefs,JSON.parse(String(init?.body||'{}')));data=prefs;}
  else if(path==='/api/v1/codex/plan') {Object.assign(overview.plan,JSON.parse(String(init?.body||'{}')));data=overview.plan;}
  else if(path==='/api/v1/codex/consumption') {
    const plan=overview.plan.plan_type;
    const five=plan==='plus';
    const duration=Number(url.searchParams.get('window')||10080);
    const granularity=url.searchParams.get('granularity');
    if(previewUsageMode==='loading') await new Promise(resolve=>setTimeout(resolve,2500));
    if(previewUsageMode==='error'||(previewUsageMode==='partial'&&granularity==='hourly')) return new Response('{}',{status:503});
    const points=Array.from({length:12},(_,i)=>({t:iso((i-11)*900000),used:(duration===300?6:3)+i*(duration===300?2:1)}));
    const series=Array.from({length:30},(_,i)=>({bucket_start:iso((i-29)*86400000).slice(0,10),consumed_pp:(i===24||i===26)?null:1.2+(i%5)*(duration===300?2:0.7),level_end:14,peak_pp_hour:1.4,resets:0,coverage_hours:i===24||i===26?0:24}));
    data={plan_type:plan,generated_at:iso(),tz_offset_minutes:480,
      windows:[...(five?[{id:'codex',duration_minutes:300,present:true}]:[]),{id:'codex',duration_minutes:10080,present:true}],
      generations:duration===300&&!five?[]:[{resets_at:Math.floor(now/1000)+(duration===300?12600:446400),start:points[0].t,end:points[11].t,points}],
      series:granularity==='monthly'?[{...series[0],bucket_start:iso().slice(0,7)+'-01',consumed_pp:130}]:series,
      hourly:Array.from({length:24},(_,i)=>({hour:iso((i-23)*3600000),consumed_pp:0.2+(i%5)*0.15,samples:12,resets:0})),
      coverage:{first_day:series[0].bucket_start,days_with_data:30},compare:{this_pp:22,prev_pp:28,ratio:22/28},reference_pp:100/7};
    if(previewUsageMode==='empty') data={plan_type:plan,generated_at:iso(),tz_offset_minutes:480,hourly:[],series:[],generations:[]};
  }
  else if(path.includes('/codex/bridges')) data={bridges:[]};
  else if(path.includes('/dashboard')) data={accounts:[],counts:{},generated_at:iso()};
  else if(path.includes('/devices')) data={devices:[]};
  else if(path.includes('/notifications')) data={notifications:[]};
  else if(path.includes('/rules')) data={rules:[]};
  else return new Response(JSON.stringify({error:{message:'界面预览不连接真实服务'}}),{status:503,headers:{'Content-Type':'application/json'}});
  return new Response(JSON.stringify(data),{headers:{'Content-Type':'application/json'}});
};
function PreviewControls(){const [mode,setMode]=useState(previewUsageMode);return <div className="preview-notice preview-controls"><span>演示数据</span><select aria-label="预览数据状态" value={mode} onChange={e=>{previewUsageMode=e.target.value;setMode(e.target.value);}}><option value="ready">正常</option><option value="empty">无样本</option><option value="error">请求失败</option><option value="partial">部分失败</option><option value="loading">慢速加载</option></select><span>切换后刷新数据</span></div>;}
const {default:App} = await import('./App');
createRoot(document.getElementById('root')!).render(<MemoryRouter initialEntries={['/codex'+(previewParams.get('view')==='usage'?'?tab=usage':'')]}><PreviewControls/><App/></MemoryRouter>);
