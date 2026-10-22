const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const defaults = {enabled:true,near_hours:24,spare_percent:35,low_percent:15,pace_lead:20,credit_hours:24,cooldown_hours:6,quiet_start:23,quiet_end:8,utc_offset_minutes:480};
function read(file, fallback=null) { try { return JSON.parse(fs.readFileSync(file,'utf8')); } catch { return fallback; } }
function write(file,value) {fs.mkdirSync(path.dirname(file),{recursive:true});const tmp=file+'.'+crypto.randomUUID()+'.tmp';fs.writeFileSync(tmp,JSON.stringify(value),{mode:0o600});fs.renameSync(tmp,file);}
function directory(root,server,user) {return path.join(root,'local-data',crypto.createHash('sha256').update(server+'|'+user).digest('hex').slice(0,24));}
function quiet(p,now=Date.now()) { const h=new Date(now+p.utc_offset_minutes*60000).getUTCHours();return p.quiet_start!==p.quiet_end&&(p.quiet_start<p.quiet_end?h>=p.quiet_start&&h<p.quiet_end:h>=p.quiet_start||h<p.quiet_end); }
function preferences(dir) {return read(path.join(dir,'preferences.json'),{preferences:defaults,plan:'unknown'});}
function settings(dir,patch) {const file=path.join(dir,'sync-settings.json');const old=read(file,{auto_sync:false});if(patch!==undefined){if(typeof patch.auto_sync!=='boolean')throw Error('无效的同步设置');write(file,{...old,auto_sync:patch.auto_sync});}return read(file,old);}
function seed(dir,overview,consumption) {
 fs.mkdirSync(dir,{recursive:true});
 if(!fs.existsSync(path.join(dir,'preferences.json')))write(path.join(dir,'preferences.json'),{preferences:overview?.preferences||defaults,plan:overview?.plan?.plan_type||'unknown'});
 if(overview){
  write(path.join(dir,'cloud-overview.json'),overview);
  if(!fs.existsSync(path.join(dir,'synced-preferences.json')))write(path.join(dir,'synced-preferences.json'),{preferences:overview.preferences,plan:overview.plan?.plan_type||'unknown'});
  const file=path.join(dir,'reminders.json'),ledger=read(file,{});
  for(const alert of overview.alerts||[])if(alert.advice?.key){const id='local:'+alert.advice.key;if(!ledger[id])ledger[id]={...alert,id,first_seen:alert.first_seen||new Date().toISOString()};}
  write(file,ledger);
 }
 if(consumption&&!fs.existsSync(path.join(dir,'baseline.json')))write(path.join(dir,'baseline.json'),consumption);
}
function overview(dir) {
 const report=read(path.join(dir,'codex-report.json')),cloud=read(path.join(dir,'cloud-overview.json'));
 if(!report)return cloud;
 const now=Date.now(),prefs=preferences(dir),paused=quiet(prefs.preferences,now);
 const ledgerFile=path.join(dir,'reminders.json'),ledger=read(ledgerFile,{});
 const alerts=[];
 const fresh=now-Date.parse(report.snapshot.observed_at)<11*60000;
 if(fresh&&prefs.preferences.enabled) for(const advice of report.analysis.advice||[]) {
  const id='local:'+advice.key;
  const state=ledger[id]||{id,first_seen:new Date().toISOString(),notified_at:null,dismissed:false,read_at:null,snoozed_until:null};
  if(!state.notified_at&&!state.dismissed&&!state.read_at&&(!state.snoozed_until||Date.parse(state.snoozed_until)<=now)&&advice.interrupt&&!paused)state.notified_at=new Date().toISOString();
  state.advice=advice;ledger[id]=state;alerts.push({...state});
 }
 // Keep receipts for the whole cycle, including transient recoveries; bound old cycles.
 // Keep compact delivery history across all cycles.
 write(ledgerFile,ledger);
 const news=[report.snapshot.news,cloud?.news].filter(Boolean).sort((a,b)=>Date.parse(b.checked_at)-Date.parse(a.checked_at))[0];
 return {generated_at:new Date().toISOString(),preferences:prefs.preferences,quiet:paused,plan:{plan_type:prefs.plan,source_type:'manual'},alerts,news,devices:[{...report,id:'local',name:'此电脑 · 本地记录',analysis:fresh?report.analysis:{state:'stale',metrics:[],advice:[]}}]};
}
function consumption(dir,query) {
 const params=new URLSearchParams(query),duration=Number(params.get('window')||10080),kind=params.get('granularity')||'daily';
 const prefs=preferences(dir),offset=prefs.preferences.utc_offset_minutes;
 const state=read(path.join(dir,'rollups.json'),{rows:{}}),baseline=read(path.join(dir,'baseline.json'),{});
 const base=baseline[duration+':'+kind]||{};
 const hours=new Map();
 for(const row of Object.values(state.rows))if(row.duration===duration){const old=hours.get(row.hour)||{hour:row.hour,consumed_pp:0,samples:0,resets:0};old.consumed_pp+=row.consumed_pp;old.samples+=row.samples;old.resets+=row.resets;hours.set(row.hour,old);}
 const observed=[state.last,base.data_as_of].filter(v=>v&&Number.isFinite(Date.parse(v))&&Date.parse(v)>0).sort((a,b)=>Date.parse(b)-Date.parse(a))[0];
 const out={generated_at:new Date().toISOString(),data_as_of:observed,source:state.last?'local_backup':base.source||'none',stale_reason:!observed?'no_source':Date.now()-Date.parse(observed)>(kind==='daily'?36*3600000:11*60000)?'stale_source':'',tz_offset_minutes:offset};
 if(kind==='hourly') {
  const all=new Map((base.hourly||[]).map(r=>[r.hour,{...r}]));
  for(const [key,row]of hours){const old=all.get(key);all.set(key,{...row,consumed_pp:row.consumed_pp+(old?.consumed_pp||0),samples:row.samples+(old?.samples||0),resets:row.resets+(old?.resets||0)});}
  out.hourly=[...all.values()].filter(r=>Date.parse(r.hour)>=Date.now()-24*3600000).sort((a,b)=>a.hour.localeCompare(b.hour));
 } else if(kind==='window') {
  const report=read(path.join(dir,'codex-report.json'));
  const groups=new Map();
  for(const sample of report?.history||[])for(const b of sample.buckets||[])for(const w of [b.primary,b.secondary])if(w?.duration_minutes===duration&&sample.status==='ok'){const key=b.id+':'+w.resets_at;const group=groups.get(key)||{resets_at:w.resets_at,points:[]};group.points.push({t:sample.observed_at,used:w.used_percent});groups.set(key,group);}
  out.generations=[...groups.values()];
  if(!out.generations.length)out.generations=base.generations||[];
 } else {
  const days=new Map((base.series||[]).map(r=>[r.bucket_start,{...r}]));
  const coverage=new Map();
  const baselineHours=new Set((baseline[duration+':hourly']?.hourly||[]).filter(r=>r.samples>0).map(r=>Date.parse(r.hour)));
  for(const row of hours.values()) {const day=new Date(Date.parse(row.hour)+offset*60000).toISOString().slice(0,10);const d=days.get(day)||{bucket_start:day,consumed_pp:0,coverage_hours:0,resets:0};d.consumed_pp=(d.consumed_pp||0)+row.consumed_pp;d.resets=(d.resets||0)+row.resets;days.set(day,d);const set=coverage.get(day)||new Set();set.add(row.hour);coverage.set(day,set);}
  for(const [day,set]of coverage){const overlap=[...set].filter(hour=>baselineHours.has(Date.parse(hour))).length;days.get(day).coverage_hours=Math.min(24,(days.get(day).coverage_hours||0)+set.size-overlap);}
  out.series=[...days.values()].sort((a,b)=>a.bucket_start.localeCompare(b.bucket_start));
 }
 return out;
}
function backup(dir,operationId) {const data={};for(const duration of [10080,300])for(const kind of ['hourly','daily',...(duration===300?['window']:[])])data[duration+':'+kind]=consumption(dir,`window=${duration}&granularity=${kind}`);const report=read(path.join(dir,'codex-report.json'));const seen=new Set();const recent_snapshots=[...(report?.history||[]),...(report?.snapshot?[report.snapshot]:[])].filter(v=>{const t=Date.parse(v.observed_at);if(!Number.isFinite(t)||Date.now()-t>15*60000||t>Date.now()+120000||seen.has(t)||v.status!=='ok')return false;seen.add(t);return true;}).sort((a,b)=>Date.parse(a.observed_at)-Date.parse(b.observed_at)).slice(-16).map(v=>({observed_at:v.observed_at,source:v.source,status:v.status,buckets:v.buckets}));const out={version:1,generated_at:new Date().toISOString(),consumption:data,recent_snapshots};if(operationId)out.operation_id=String(operationId).slice(0,128);return out;}
// Durable upload queue: one pending snapshot reception at a time. The entry
// survives restarts; retries reuse the same operation id so the server can
// deduplicate a lost-response replay.
const QUEUE_FILE='upload-queue.json';
function queueRead(dir){return read(path.join(dir,QUEUE_FILE),{epoch:null,sequence:0,pending:null});}
function queueEnqueue(dir){const q=queueRead(dir);if(typeof q.epoch!=='string'||!q.epoch)q.epoch=crypto.randomUUID();q.sequence=(Number.isInteger(q.sequence)?q.sequence:0)+1;const entry={id:q.epoch+'-'+q.sequence,created_at:new Date().toISOString(),attempts:0,next_attempt:null,needs_auth:false,last_error:null};write(path.join(dir,QUEUE_FILE),{...q,pending:entry});return {...entry};}
function queueStatus(dir){const q=queueRead(dir),p=q.pending;return {pending:!!p,operation_id:p?p.id:null,created_at:p?p.created_at:null,attempts:p?p.attempts:0,next_attempt:p?p.next_attempt:null,needs_auth:!!(p&&p.needs_auth),last_error:p?p.last_error:null,sequence:Number.isInteger(q.sequence)?q.sequence:0};}
function queueAccept(dir,id){const q=queueRead(dir);if(q.pending&&q.pending.id===id)write(path.join(dir,QUEUE_FILE),{...q,pending:null});}
function queueFail(dir,id,error,{backoff_ms=0,needs_auth=false}={}){const q=queueRead(dir);if(!q.pending||q.pending.id!==id)return;const attempts=(q.pending.attempts||0)+1;q.pending={...q.pending,attempts,last_error:String(error||'上传失败').slice(0,300),needs_auth:!!needs_auth,next_attempt:backoff_ms>0?new Date(Date.now()+backoff_ms).toISOString():null};write(path.join(dir,QUEUE_FILE),{...q,pending:q.pending});}
function dashboard(dir){
 const result=read(path.join(dir,'cloud-dashboard.json'),{accounts:[],counts:{}}),sample=read(path.join(dir,'cursor-latest.json'));
 if(!sample)return result;
 const previous=result.accounts.find(a=>a.provider?.slug==='cursor');
 const fresh=sample.status==='ok'&&Date.now()-Date.parse(sample.observed_at)<3*60000;
 const buckets=(sample.buckets?.length?sample.buckets:[{scope_key:'default',used_percent:sample.used_percent}]).map(b=>{
  const old=previous?.buckets?.find(x=>x.scope_key===b.scope_key);
  const ratio=typeof b.used_percent==='number'?(100-b.used_percent)/100:null;
  return {...old,id:old?.id||'local-cursor-'+b.scope_key,scope_key:b.scope_key,unit:'percent',quota_type:'usage',limit_value:100,remaining_value:ratio==null?null:ratio*100,remaining_ratio:ratio,used_value:b.used_percent,reset_at:sample.reset_at||null,expires_at:null,observed_at:sample.observed_at,source_type:'cursor_api2',collection_status:fresh?'ok':'stale',status:!fresh?'stale':ratio!=null&&ratio<=0.2?'low':'healthy',confidence:'high',reset_policy:'billing_cycle',note:'保存在本机'};
 });
 const account={...previous,id:previous?.id||'local-cursor',display_name:'Cursor 本机',provider:{id:'prov_cursor',slug:'cursor',display_name:'Cursor'},plan_name:sample.plan_name||'Cursor',plan_code:sample.plan_name||'',computed_status:fresh?'healthy':'stale',buckets};
 return {...result,generated_at:new Date().toISOString(),accounts:[...result.accounts.filter(a=>a.provider?.slug!=='cursor'),account]};
}
function notifications(dir){
 const base=read(path.join(dir,'cloud-notifications.json'),{notifications:[]});
 const rules=read(path.join(dir,'notification-rules.json'),{rules:[]}).rules;
 const file=path.join(dir,'reminders.json'),ledger=read(file,{}),result=[];
 for(const account of dashboard(dir).accounts.filter(a=>a.provider?.slug==='cursor'))for(const b of account.buckets){
  const activeRules=rules.filter(r=>r.enabled&&r.rule_type==='quota.low'&&(!r.provider_account_id||r.provider_account_id===account.id));
  const threshold=rules.length?Math.max(0,...activeRules.map(r=>r.params?.ratio??0.2)):0.2;
  if(b.collection_status==='stale'||b.remaining_ratio==null||b.remaining_ratio>=threshold)continue;
  const dedupe='quota.low:'+b.id+(b.reset_at?':'+new Date(b.reset_at).toISOString().replace(/\.\d{3}Z$/,'Z'):'');
  const id='local-cursor:'+dedupe+(b.remaining_ratio<=0?':exhausted':':low');
  const a=ledger[id]||{id,first_seen:new Date().toISOString(),read_at:null,dismissed:false,snoozed_until:null};
  a.dedupe_key=dedupe;ledger[id]=a;
  result.push({id,title:b.remaining_ratio<=0?'Cursor 当前额度已用完':'Cursor 剩余额度偏低',body:`${b.scope_key} 剩余 ${(b.remaining_ratio*100).toFixed(1)}%。${b.reset_at?'重置时间 '+new Date(b.reset_at).toLocaleString():'重置时间未知'}`,severity:b.remaining_ratio<=0?'critical':'warning',status:a.dismissed?'dismissed':a.read_at?'read':a.snoozed_until&&Date.parse(a.snoozed_until)>Date.now()?'snoozed':'unread',created_at:a.first_seen,delivery_token:a.snoozed_until||a.first_seen,active:true,provider_slug:'cursor',provider_name:'Cursor',dedupe_key:dedupe});
 }
 write(file,ledger);
 return {notifications:[...result,...base.notifications.filter(n=>!(n.provider_slug==='cursor'&&n.dedupe_key?.startsWith('quota.low:')))]};
}
function request(dir,pathname,method='GET',body) {
 const [route,query='']=pathname.split('?');
 if(route==='/api/v1/dashboard'&&method==='GET')return dashboard(dir);
 if(route==='/api/v1/notifications'&&method==='GET')return notifications(dir);
 const generic=route.match(/^\/api\/v1\/notifications\/([^/]+)\/action$/);
 if(generic&&method==='POST'&&decodeURIComponent(generic[1]).startsWith('local-cursor:')){
  const file=path.join(dir,'reminders.json'),ledger=read(file,{}),entry=ledger[decodeURIComponent(generic[1])];
  if(!entry||!['read','dismiss','snooze','restore'].includes(body?.action))throw Error('提醒已变化');
  entry.read_at=body.action==='read'?new Date().toISOString():null;entry.dismissed=body.action==='dismiss';entry.snoozed_until=body.action==='snooze'?new Date(Date.now()+3600000).toISOString():null;entry.pending_action=body.action;write(file,ledger);return {ok:true};
 }
 if(route==='/api/v1/codex/overview'&&method==='GET')return overview(dir);
 if(route==='/api/v1/codex/consumption'&&method==='GET')return consumption(dir,query);
 if(route==='/api/v1/codex/plan') {
  const prefs=preferences(dir);
  if(method==='PUT'){if(!['plus','pro','prolite','unknown'].includes(body?.plan_type))throw Error('无效的套餐');prefs.plan=body.plan_type;write(path.join(dir,'preferences.json'),prefs);}
  return {plan_type:prefs.plan,source_type:'manual'};
 }
 if(route==='/api/v1/codex/preferences'&&method==='PATCH') {
  const p=body;
  const ranges={near_hours:[1,168],spare_percent:[10,95],low_percent:[1,30],pace_lead:[5,70],credit_hours:[1,168],cooldown_hours:[1,48],quiet_start:[0,23],quiet_end:[0,23],utc_offset_minutes:[-720,840]};
  if(!p||typeof p.enabled!=='boolean'||!Object.entries(ranges).every(([k,[min,max]])=>Number.isInteger(p[k])&&p[k]>=min&&p[k]<=max)||p.spare_percent<=p.low_percent)throw Error('无效的提醒规则');
  write(path.join(dir,'preferences.json'),{...preferences(dir),preferences:p});return p;
 }
 const match=route.match(/^\/api\/v1\/codex\/alerts\/([^/]+)(\/delivery)?$/);
 if(match&&method==='POST') {
  const id=decodeURIComponent(match[1]);if(!id.startsWith('local:'))return undefined;
  const file=path.join(dir,'reminders.json'),all=read(file,{}),alert=all[id];if(!alert)return {claimed:false,reason:'inactive'};
  if(match[2]) {
   if(body.action==='claim') {
    if(alert.read_at||alert.dismissed||quiet(preferences(dir).preferences)||!preferences(dir).preferences.enabled||alert.notified_at!==body.token)return {claimed:false,reason:'changed'};
    if(alert.delivered===body.token)return {claimed:false,reason:'delivered'};
    if(alert.lease>Date.now())return {claimed:false,reason:'reserved'};
    alert.claim_id=crypto.randomUUID();alert.lease=Date.now()+120000;write(file,all);return {claimed:true,claim_id:alert.claim_id};
   }
   if(alert.claim_id!==body.claim_id)throw Error('提醒已变化');
   if(body.action==='delivered')alert.delivered=alert.notified_at;
   else if(body.action==='release')delete alert.delivered;
   else throw Error('无效的提醒操作');
   alert.lease=0;
  } else {
   if(!['read','dismiss','snooze','restore'].includes(body.action))throw Error('无效的提醒操作');
   alert.read_at=body.action==='read'?new Date().toISOString():null;
   alert.dismissed=body.action==='dismiss';alert.snoozed_until=body.action==='snooze'?new Date(Date.now()+3600000).toISOString():null;
   // Presentation records are permanent, independent of list actions.
   alert.pending_action=body.action;
  }
  write(file,all);return {ok:true};
 }
 return undefined;
}
module.exports={directory,read,write,settings,seed,overview,consumption,backup,request,preferences,queueEnqueue,queueStatus,queueAccept,queueFail};
