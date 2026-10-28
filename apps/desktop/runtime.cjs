const {app,safeStorage}=require('electron');
const {spawn}=require('node:child_process');
const crypto=require('node:crypto');
const fs=require('node:fs');
const https=require('node:https');
const path=require('node:path');
const platform=require('./platform.cjs');
const credentialStore=require('./credentialStore.cjs');
const localData=require('./localData.cjs');
const desktopBridge=require('./desktopBridge.cjs');
const secretCache=new Map();

const LOCAL_ORIGIN='http://127.0.0.1:8080';
const DEFAULT_CLOUD='http://127.0.0.1:8080';
let lastError='';
let managed={server:null,bridge:null};
let bridgeSync=null;
let cachedConfig=null;
let activeMode=null;

function readJson(file){try{return JSON.parse(fs.readFileSync(file,'utf8'));}catch{return null;}}
function dataDir(){return path.join(app.getPath('userData'),'runtime');}
function ensureDataDir(){const dir=dataDir();fs.mkdirSync(dir,{recursive:true});return dir;}
function secretPath(name){return path.join(ensureDataDir(),name+'.protected');}
function readSecret(name){try{
 const file=secretPath(name),stat=fs.statSync(file),stamp=stat.mtimeMs+':'+stat.size;
 if(secretCache.get(name)?.stamp===stamp)return secretCache.get(name).value;
 const value=credentialStore.decrypt(fs.readFileSync(file),safeStorage);
 secretCache.set(name,{stamp,value});return value;
}catch(error){
 if(fs.existsSync(secretPath(name)))lastError='系统凭据读取失败，请重新登录以恢复连接';
 if(process.env.AIHUB_STARTUP_DIAGNOSTICS==='1')console.error('credential_read_failed',name,error.message);
 return '';
}}
function writeSecret(name,value){
 const file=secretPath(name),temporary=file+'.tmp-'+crypto.randomUUID();
 try{fs.writeFileSync(temporary,credentialStore.encrypt(value,safeStorage),{mode:0o600});fs.renameSync(temporary,file);}
 finally{if(fs.existsSync(temporary))fs.unlinkSync(temporary);}
 secretCache.delete(name);
}
function extensionPairCode(){let code=readSecret('extension-pair');if(!code){code=crypto.randomBytes(24).toString('hex');writeSecret('extension-pair',code)}return code;}
function readDesktopSession(profileId){const all=readSecret('sessions');try{return JSON.parse(all||'{}')[profileId]||null;}catch{return null;}}
function writeDesktopSession(profileId,session){let all={};try{all=JSON.parse(readSecret('sessions')||'{}');}catch{}if(session)all[profileId]=session;else delete all[profileId];writeSecret('sessions',JSON.stringify(all));}
function binPath(kind){
 const names=platform.binNames();
 if(!app.isPackaged){return path.join(__dirname,'..','..','artifacts','aihub-m0',kind==='bridge'?names.bridgeDev:names.serverDev);}
 return path.join(process.resourcesPath,'bin',kind==='bridge'?names.bridgePackaged:names.serverPackaged);
}
function bundledConfig(){
 const file=app.isPackaged?path.join(process.resourcesPath,'app-config.json'):path.join(__dirname,'resources','app-config.json');
 return readJson(file)||{};
}
function loadConfig(dir){
 if(cachedConfig)return cachedConfig;
 const cfg={uiMode:'ask',cloudServer:DEFAULT_CLOUD,profileId:'public-cloud',profileName:'公网 AI Hub',bridgeIntervalSeconds:60,...bundledConfig(),...readJson(path.join(dir,'app-config.json'))};
 cachedConfig=cfg;
 return cfg;
}
function setCloudServer(input){
 const u=new URL(input);
 if(u.username||u.password||u.search||u.hash||u.pathname!=='/'||!(u.protocol==='https:'||(u.protocol==='http:'&&['localhost','127.0.0.1','[::1]'].includes(u.hostname))))throw Error('请输入 HTTPS 服务器根地址；本机开发可使用 localhost HTTP');
 const dir=ensureDataDir(),file=path.join(dir,'app-config.json');
 fs.writeFileSync(file,JSON.stringify({...readJson(file),cloudServer:u.origin,uiMode:'cloud'},null,2),{mode:0o600});
 cachedConfig=null;activeMode='cloud';return u.origin;
}
function getActiveMode(config=loadConfig(ensureDataDir())){
 if(activeMode)return activeMode;
 if(config.uiMode==='local')return 'local';
 if(config.uiMode==='cloud')return 'cloud';
 return 'cloud';
}
function getUiOrigin(config=loadConfig(ensureDataDir())){
 return getActiveMode(config)==='local'?LOCAL_ORIGIN:(config.cloudServer||DEFAULT_CLOUD).replace(/\/$/,'');
}
function prefPath(){return path.join(ensureDataDir(),'mode-preference.json');}
function installationID(previous){
 const file=path.join(ensureDataDir(),'installation.json');
 const saved=readJson(file);
 if(typeof saved?.id==='string'&&saved.id.length>=16)return saved.id;
 const id=typeof previous==='string'&&/^[a-f0-9-]{36}$/i.test(previous)?previous:crypto.randomUUID();
 fs.writeFileSync(file,JSON.stringify({id}),{mode:0o600});return id;
}
function readModePreference(){return readJson(prefPath());}
function clearModePreference(){try{fs.unlinkSync(prefPath());}catch{} resetMode();}
function setUserMode(mode){
 activeMode=mode==='cloud'?'cloud':'local';
 cachedConfig=null;
 const dir=ensureDataDir();
 const prev=readJson(path.join(dir,'app-config.json'))||{};
 fs.writeFileSync(path.join(dir,'app-config.json'),JSON.stringify({...prev,uiMode:activeMode},null,2));
}
function resetMode(){activeMode=null;cachedConfig=null;}
function forceLocalMode(){
 activeMode='local';
 cachedConfig=null;
 const dir=ensureDataDir();
 const prev=readJson(path.join(dir,'app-config.json'))||{};
 fs.writeFileSync(path.join(dir,'app-config.json'),JSON.stringify({...prev,uiMode:'local',cloudFallback:true},null,2));
}
function httpsJson(method,urlStr,body,headers={}){
 return new Promise((resolve,reject)=>{
  const u=new URL(urlStr);
  const data=body?JSON.stringify(body):null;
  const req=https.request({hostname:u.hostname,port:u.port||443,path:u.pathname+u.search,method,headers:{'Content-Type':'application/json',...headers,...(data?{'Content-Length':Buffer.byteLength(data)}:{})},timeout:15000,servername:u.hostname},res=>{
   let raw='';
   res.on('data',c=>{raw+=c;});
   res.on('end',()=>{try{const parsed=JSON.parse(raw||'{}');if(res.statusCode>=300){const err=new Error(parsed.message||parsed.error||`HTTP ${res.statusCode}`);err.status=res.statusCode;reject(err);}else resolve(parsed);}catch(e){reject(e);}});
  });
  req.on('error',reject);
  req.setTimeout(15000,()=>req.destroy(new Error('连接超时')));
  if(data)req.write(data);
  req.end();
 });
}
async function probeCloud(server){
 const base=(server||DEFAULT_CLOUD).replace(/\/$/,'');
 try{
  if(base.startsWith('https://'))return await httpsJson('GET',`${base}/ready`).then(r=>r.status==='ok');
  const res=await fetch(`${base}/ready`,{signal:AbortSignal.timeout(8000)});
  if(!res.ok)return false;
  const body=await res.json();
  return body.status==='ok';
 }catch{return false;}
}
async function resolveActiveMode(dir,config){
 if(activeMode)return activeMode;
 if(config.uiMode==='local'){activeMode='local';return activeMode;}
 if(config.uiMode==='cloud'){activeMode='cloud';return activeMode;}
 if(config.uiMode==='ask')throw new Error('请先选择「登录服务器」或「离线本地模式」');
 if(config.uiMode==='auto'){
  activeMode=(await probeCloud(config.cloudServer))?'cloud':'local';
  if(activeMode==='local')lastError='公网暂不可达，已自动切换本地模式';
  return activeMode;
 }
 activeMode='cloud';
 return activeMode;
}
async function waitReady(origin,ms=30000){
 const start=Date.now();
 while(Date.now()-start<ms){
  try{
   const res=await fetch(`${origin}/ready`,{signal:AbortSignal.timeout(1500)});
   if(res.ok){const body=await res.json();if(body.status==='ok')return true;}
  }catch{}
  await new Promise(r=>setTimeout(r,500));
 }
 return false;
}
function spawnHidden(exe,args,logBase,extraEnv={}){
 const out=path.join(logBase,path.basename(exe)+'.log');
 const err=path.join(logBase,path.basename(exe)+'-error.log');
 const child=spawn(exe,args,{cwd:path.dirname(exe),env:{...process.env,...extraEnv},stdio:['ignore','pipe','pipe'],windowsHide:true,detached:false});
 child.stdout?.on('data',d=>fs.appendFileSync(out,d));
 child.stderr?.on('data',d=>fs.appendFileSync(err,d));
 return child;
}
async function ensureServer(dir){
 if(await waitReady(LOCAL_ORIGIN,1500))return;
 const serverExe=binPath('server');
 if(!fs.existsSync(serverExe))throw new Error('缺少内置服务端程序');
 const dbUrl=await platform.ensureDatabase(dir);
 const pidFile=path.join(dir,'server.pid');
 if(fs.existsSync(pidFile)){
  try{process.kill(Number(fs.readFileSync(pidFile,'utf8').trim()),0);if(await waitReady(LOCAL_ORIGIN,3000))return;}catch{}
 }
 const child=spawnHidden(serverExe,[],dir,{AIHUB_DATABASE_URL:dbUrl,AIHUB_ADDR:'127.0.0.1:8080'});
 fs.writeFileSync(pidFile,String(child.pid));
 managed.server=child;
 child.on('exit',()=>{if(managed.server===child)managed.server=null;});
 if(!await waitReady(LOCAL_ORIGIN,45000))throw new Error(platform.serverStartupHint(dir)||'本地服务启动超时，请确认数据库已就绪');
}
function bridgeReady(cfg){return cfg&&cfg.server&&cfg.device_id&&readSecret('bridge-token');}
async function apiJson(url,options={}){
 const u=new URL(url);
 if(u.protocol==='https:'){
  const body=options.body?JSON.parse(options.body):undefined;
  const headers=options.headers||{};
  return httpsJson(options.method||'GET',url,body,headers);
 }
 const res=await fetch(url,{...options,headers:{'Content-Type':'application/json',...(options.headers||{})},signal:AbortSignal.timeout(20000)});
 const parsed=await res.json().catch(()=>({}));
 if(!res.ok){const err=new Error(parsed.message||parsed.error||`请求失败 (${res.status})`);err.status=res.status;throw err;}
 return parsed;
}
async function createBridgeToken(server,accessToken,name){
 const out=await apiJson(`${server.replace(/\/$/,'')}/api/v1/codex/bridges`,{method:'POST',headers:{Authorization:`Bearer ${accessToken}`},body:JSON.stringify({name})});
 return out.token;
}
function writeBridge(dir,cfg,token){writeSecret('bridge-token',token);fs.writeFileSync(path.join(dir,'bridge.json'),JSON.stringify(cfg,null,2),{mode:0o600});}
function ensureBridgeConfig(dir,config){
 const cfgPath=path.join(dir,'bridge.json');
 if(fs.existsSync(cfgPath)){
  const existing=readJson(cfgPath);
  if(existing?.token){delete existing.token;fs.writeFileSync(cfgPath,JSON.stringify(existing,null,2),{mode:0o600});}
  return cfgPath;
 }
 const example=app.isPackaged?path.join(process.resourcesPath,'bridge.example.json'):path.join(__dirname,'resources','bridge.example.json');
 const seed=readJson(example)||{server:config.cloudServer||DEFAULT_CLOUD,codex_path:'',cursor_enabled:true,interval_seconds:60};
 delete seed.token;
 fs.writeFileSync(cfgPath,JSON.stringify(seed,null,2));
 return cfgPath;
}
async function ensureBridge(dir,config){
 ensureBridgeConfig(dir,config);
 const cfgPath=path.join(dir,'bridge.json');
 if(desktopBridge.isPaused(dir))return;
 let cfg=readJson(cfgPath);
 if(cfg&&cfg.cursor_enabled!==true){cfg.cursor_enabled=true;fs.writeFileSync(cfgPath,JSON.stringify(cfg,null,2),{mode:0o600});}
 if(!bridgeReady(cfg))return;
 if(!cfg.codex_path||!fs.existsSync(cfg.codex_path)){
  const found=await platform.findCodex();
  cfg.codex_path=found||'';
  fs.writeFileSync(cfgPath,JSON.stringify(cfg,null,2),{mode:0o600});
 }
 const bridgeExe=binPath('bridge');
 if(!fs.existsSync(bridgeExe))return;
 const stateFile=path.join(dir,'bridge-process.json');
 const configHash=crypto.createHash('sha256').update(fs.readFileSync(cfgPath)).digest('hex');
 if(fs.existsSync(stateFile)){
  try{
   const saved=JSON.parse(fs.readFileSync(stateFile,'utf8'));
   process.kill(saved.Id,0);
   if(saved.Binary===bridgeExe&&saved.ConfigHash===configHash)return;
   if(managed.bridge?.pid===saved.Id)stopBridge();
   else if(saved.Binary===bridgeExe){process.kill(saved.Id);fs.unlinkSync(stateFile);}
  }catch{if(fs.existsSync(stateFile))fs.unlinkSync(stateFile);}
 }
 const child=spawnHidden(bridgeExe,['--config',cfgPath],dir,{AIHUB_BRIDGE_TOKEN:readSecret('bridge-token'),AIHUB_EXTENSION_PAIR_CODE:extensionPairCode()});
 fs.writeFileSync(stateFile,JSON.stringify({Id:child.pid,Binary:bridgeExe,ConfigHash:configHash}));
 managed.bridge=child;
 child.on('exit',(code)=>{if(managed.bridge===child){managed.bridge=null;try{fs.unlinkSync(stateFile);}catch{}} if(code)lastError=`Codex 采集器退出 (${code})`;});
}
async function ensureReady(){
 if(process.env.AIHUB_DESKTOP_SKIP_RUNTIME==='1')return;
 try{
  const dir=ensureDataDir();
  const config=loadConfig(dir);
  await resolveActiveMode(dir,config);
  if(getActiveMode(config)==='local'){
   await platform.ensurePostgres(dir);
   await ensureServer(dir);
  }
  ensureBridgeConfig(dir,config);
 }catch(e){
  lastError=e.message||String(e);
  throw e;
 }
}
function syncBridgeFromSession(webContents,config){
 if(bridgeSync)return bridgeSync;
 bridgeSync=syncBridgeSession(webContents,config).finally(()=>{bridgeSync=null;});
 return bridgeSync;
}
// Share the renderer's refresh lock: refresh tokens are rotated and must never
// be consumed concurrently by a background sync and a normal page request.
async function refreshSessionForSync(webContents,profileId){
 async function refreshInRenderer(id){
  await navigator.locks.request('hub-refresh:'+id,async()=>{
   const session=window.aihubDesktop.sessionGet(id);
   if(!session?.token||session.blocked)throw Error('请重新登录以继续同步');
   if(Date.parse(session.expires_at)>Date.now()+60000)return;
   const response=await fetch('/api/v1/auth/refresh',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({refresh_token:session.refresh_token}),signal:AbortSignal.timeout(15000),redirect:'error'});
   if(window.aihubDesktop.sessionGet(id)?.refresh_token!==session.refresh_token)throw Error('登录状态已改变');
   if(!response.ok){if(response.status===401)window.aihubDesktop.sessionSet(id,{...session,blocked:true});throw Error('登录续期失败，本地数据已保留');}
   if(!window.aihubDesktop.sessionSet(id,await response.json()))throw Error('无法安全保存续期凭据');
  });
 }
 return webContents.executeJavaScript(`(${refreshInRenderer.toString()})(${JSON.stringify(profileId)})`,true);
}
async function syncBridgeSession(webContents,config){
 const dir=ensureDataDir();
 const picked=await webContents.executeJavaScript(`(()=>{try{const id=localStorage.getItem('hub_profile');const profiles=JSON.parse(localStorage.getItem('hub_profiles')||'[]');const profile=profiles.find(p=>p.id===id)||profiles[0]||{id:'default',url:location.origin};return {id:profile.id,server:profile.url};}catch{return null;}})()`,true);
 const session=picked?readDesktopSession(picked.id):null;
 if(process.env.AIHUB_STARTUP_DIAGNOSTICS==='1')console.error('startup_session',JSON.stringify({profile:picked?.id,hasSession:!!session,blocked:!!session?.blocked,electron:process.versions.electron}));
 if(!session?.token||session.blocked){stopBridge();return;}
 const localDir=localDirectory(picked.id);
 await seedLocalData(picked.id);
 const existing=readJson(path.join(dir,'bridge.json'));
 if(bridgeReady(existing)&&existing.server===picked.server&&existing.device_id===session.device_id){
  if(existing.local_store!==localDir){existing.local_store=localDir;fs.writeFileSync(path.join(dir,'bridge.json'),JSON.stringify(existing,null,2));}
  await ensureBridge(dir,config);
  if(localData.settings(localDir).auto_sync){try{await refreshSessionForSync(webContents,picked.id);await syncLocalData(picked.id);}catch(e){lastError=e.message;const file=path.join(localDir,'sync-status.json');localData.write(file,{...localData.read(file,{}),error:e.message});}}
  return;
 }
 stopBridge();
 try{
  const token=await createBridgeToken(picked.server,session.token,'我的电脑');
  writeBridge(dir,{server:picked.server,device_id:session.device_id,local_store:localDir,codex_path:await platform.findCodex()||'',cursor_enabled:true,interval_seconds:config.bridgeIntervalSeconds||60},token);
  await ensureBridge(dir,config);
 }catch(e){lastError=`采集器连接失败：${e.message}`;}
}
function stopBridge(){
 const child=managed.bridge;if(child&&!child.killed){try{child.kill();}catch{}}
 managed.bridge=null;
 const stateFile=path.join(ensureDataDir(),'bridge-process.json');
 const saved=readJson(stateFile);
 if(saved?.Binary===binPath('bridge')&&Number.isInteger(saved.Id)){try{process.kill(saved.Id);}catch{}}
 try{fs.unlinkSync(stateFile);}catch{}
}
function stopManaged(){
 stopBridge();
 for(const key of ['server']){
  const child=managed[key];
  if(child&&!child.killed){try{child.kill();}catch{}}
  managed[key]=null;
 }
}
function startupMessage(){return lastError||'';}
function localDirectory(profileId){const session=readDesktopSession(profileId);if(!session?.user?.id||session.blocked)throw Error('请先登录');return localData.directory(app.getPath('userData'),getUiOrigin(),session.user.id);}
async function localRequest(profileId,pathname,method,body){
 if(typeof pathname!=='string'||pathname.length>1024||!['GET','POST','PATCH','PUT'].includes(method))throw Error('无效的本地请求');
 const dir=localDirectory(profileId);
 if(pathname==='/api/v1/notifications'&&method==='GET'){
  const file=path.join(dir,'notification-rules.json');
  if(!fs.existsSync(file)||Date.now()-fs.statSync(file).mtimeMs>60000){try{const session=readDesktopSession(profileId);localData.write(file,await apiJson(getUiOrigin()+'/api/v1/notification-rules',{headers:{Authorization:`Bearer ${session.token}`}}));}catch{/* Use the last saved rules while offline. */}}
 }
 const value=localData.request(dir,pathname,method,body);return {handled:value!==undefined&&value!==null,value};
}
function localStatus(profileId,patch){const dir=localDirectory(profileId);return {...localData.settings(dir,patch),...localData.read(path.join(dir,'sync-status.json'),{}),directory:dir,observed_at:localData.read(path.join(dir,'codex-latest.json'))?.observed_at,queue:localData.queueStatus(dir)};}
async function seedLocalData(profileId){
 const dir=localDirectory(profileId);if(fs.existsSync(path.join(dir,'baseline.json')))return;
 const session=readDesktopSession(profileId),headers={Authorization:`Bearer ${session.token}`},server=getUiOrigin();
 try {
  const overview=await apiJson(server+'/api/v1/codex/overview',{headers});
  localData.write(path.join(dir,'cloud-dashboard.json'),await apiJson(server+'/api/v1/dashboard',{headers}));
  localData.write(path.join(dir,'cloud-notifications.json'),await apiJson(server+'/api/v1/notifications',{headers}));
  localData.write(path.join(dir,'notification-rules.json'),await apiJson(server+'/api/v1/notification-rules',{headers}));
  const data={};
  for(const duration of [10080,300])for(const kind of ['daily','hourly',...(duration===300?['window']:[])])data[duration+':'+kind]=await apiJson(server+`/api/v1/codex/consumption?window=${duration}&granularity=${kind}&days=365&hours=24`,{headers});
  localData.seed(dir,overview,data);
 }catch{localData.seed(dir,null,null);}
}
const localSyncs=new Map();
// Durable upload queue for local-data reception (plan 4.3): retries reuse the
// same operation id, so a retry after a lost response replays as a duplicate
// on the server instead of a second reception.
const UPLOAD_BACKOFF_MS=[30000,60000,120000,240000,480000,900000];
function uploadBackoffMs(attempts){const base=UPLOAD_BACKOFF_MS[Math.min(Math.max(attempts,0),UPLOAD_BACKOFF_MS.length-1)];return Math.round(base*(0.8+Math.random()*0.4));}
const queueDrains=new Map();
function scheduleQueueDrain(profileId,delayMs){
 let dir;try{dir=localDirectory(profileId);}catch{return;}
 if(queueDrains.has(dir))clearTimeout(queueDrains.get(dir));
 const timer=setTimeout(()=>{queueDrains.delete(dir);void drainLocalQueue(profileId).catch(()=>{});},Math.max(delayMs,1000));
 timer.unref();queueDrains.set(dir,timer);
}
async function drainLocalQueue(profileId){
 const dir=localDirectory(profileId),status=localData.queueStatus(dir);
 if(!status.pending||status.needs_auth)return;
 if(status.next_attempt&&Date.parse(status.next_attempt)>Date.now()){scheduleQueueDrain(profileId,Date.parse(status.next_attempt)-Date.now());return;}
 const session=readDesktopSession(profileId),server=getUiOrigin(),headers={Authorization:`Bearer ${session.token}`};
 const cfg=readJson(path.join(dataDir(),'bridge.json'));
 if(cfg?.local_store!==dir||cfg.server!==server)return; // bridge not ready yet; retried at the next sync
 try{
  await apiJson(server+'/api/v1/local-data',{method:'PUT',headers,body:JSON.stringify(localData.backup(dir,status.operation_id))});
  localData.queueAccept(dir,status.operation_id);
 }catch(e){
  const needsAuth=e.status===401||e.status===403;
  const backoff=needsAuth?0:uploadBackoffMs(status.attempts);
  localData.queueFail(dir,status.operation_id,e.message,{backoff_ms:backoff,needs_auth:needsAuth});
  if(!needsAuth&&localData.settings(dir).auto_sync)scheduleQueueDrain(profileId,backoff);
 }
}
function syncLocalData(profileId){
 const dir=localDirectory(profileId);if(localSyncs.has(dir))return localSyncs.get(dir);
 const task=(async()=>{
  const session=readDesktopSession(profileId),server=getUiOrigin(),headers={Authorization:`Bearer ${session.token}`};
  const cfg=readJson(path.join(dataDir(),'bridge.json'));
  if(cfg?.local_store!==dir||cfg.server!==server)throw Error('采集连接尚未就绪');
  const statusFile=path.join(dir,'sync-status.json');
  try {
   localData.write(path.join(dir,'notification-rules.json'),await apiJson(server+'/api/v1/notification-rules',{headers}));
   for(const slug of ['codex','cursor']){const snapshot=localData.read(path.join(dir,slug+'-latest.json'));if(snapshot)await apiJson(server+`/api/v1/${slug}/snapshot`,{method:'POST',headers:{Authorization:`Bearer ${readSecret('bridge-token')}`},body:JSON.stringify(snapshot)});}
   const prefs=localData.preferences(dir),previous=localData.read(path.join(dir,'synced-preferences.json'));
   const equal=(a,b)=>JSON.stringify(a)===JSON.stringify(b);
   if(!previous||!equal(prefs.preferences,previous.preferences))await apiJson(server+'/api/v1/codex/preferences',{method:'PATCH',headers,body:JSON.stringify(prefs.preferences)});
   if(!previous||prefs.plan!==previous.plan)await apiJson(server+'/api/v1/codex/plan',{method:'PUT',headers,body:JSON.stringify({plan_type:prefs.plan})});
   const entry=localData.queueEnqueue(dir);
   await apiJson(server+'/api/v1/local-data',{method:'PUT',headers,body:JSON.stringify(localData.backup(dir,entry.id))});
   localData.queueAccept(dir,entry.id);
   const remote=await apiJson(server+'/api/v1/codex/overview',{headers});
   const synchronized={preferences:remote.preferences,plan:remote.plan?.plan_type||'unknown'};
   if(equal(localData.preferences(dir),prefs))localData.write(path.join(dir,'preferences.json'),synchronized);
   localData.write(path.join(dir,'synced-preferences.json'),synchronized);
   localData.write(path.join(dir,'cloud-overview.json'),remote);
   const file=path.join(dir,'reminders.json'),ledger=localData.read(file,{});
   for(const alert of remote.alerts||[]) {
    const local=ledger['local:'+alert.advice.key];if(!local)continue;
    const before=JSON.stringify(local);
    if(local.pending_action){await apiJson(server+'/api/v1/codex/alerts/'+encodeURIComponent(alert.id),{method:'POST',headers,body:JSON.stringify({action:local.pending_action})});delete local.pending_action;}
    else {local.read_at=alert.read_at;local.dismissed=alert.dismissed;local.snoozed_until=alert.snoozed_until;}
    const current=localData.read(file,{});if(JSON.stringify(current[local.id])===before){current[local.id]=local;localData.write(file,current);}
   }
   const generic=await apiJson(server+'/api/v1/notifications',{headers});
   localData.write(path.join(dir,'cloud-notifications.json'),generic);
   localData.write(path.join(dir,'cloud-dashboard.json'),await apiJson(server+'/api/v1/dashboard',{headers}));
   for(const entry of Object.values(localData.read(file,{})).filter(a=>a.id.startsWith('local-cursor:'))){
    const target=generic.notifications.find(n=>n.active&&n.dedupe_key===entry.dedupe_key);if(!target)continue;
    const before=JSON.stringify(entry);
    if(entry.pending_action){await apiJson(server+'/api/v1/notifications/'+encodeURIComponent(target.id)+'/action',{method:'POST',headers,body:JSON.stringify({action:entry.pending_action})});delete entry.pending_action;}
    else if(target.status==='read')entry.read_at=new Date().toISOString();else if(target.status==='dismissed')entry.dismissed=true;
    const current=localData.read(file,{});if(JSON.stringify(current[entry.id])===before){current[entry.id]=entry;localData.write(file,current);}
   }
   localData.write(statusFile,{last_sync:new Date().toISOString(),error:null});
  }catch(e){
   const pending=localData.queueStatus(dir);
   if(pending.pending){
    const needsAuth=e.status===401||e.status===403;
    const backoff=needsAuth?0:uploadBackoffMs(pending.attempts);
    localData.queueFail(dir,pending.operation_id,e.message,{backoff_ms:backoff,needs_auth:needsAuth});
    if(!needsAuth&&localData.settings(dir).auto_sync)scheduleQueueDrain(profileId,backoff);
   }
   localData.write(statusFile,{...localData.read(statusFile,{}),error:e.message});throw e;
  }
  return localStatus(profileId);
 })().finally(()=>localSyncs.delete(dir));localSyncs.set(dir,task);return task;
}
// Desktop collector status, control and bounded local tooling (plan phase 2).
// Everything below stays local: the desktop tool never opens a network port
// and scan results only contain file metadata plus secret key NAMES.
function bridgeAlive(dir){
 const child=managed.bridge;
 if(child&&!child.killed)return {running:true,pid:child.pid};
 const saved=readJson(path.join(dir,'bridge-process.json'));
 if(Number.isInteger(saved?.Id)){
  try{process.kill(saved.Id,0);return {running:true,pid:saved.Id};}catch{}
 }
 return {running:false,pid:null};
}
function bridgeStatus(){
 const dir=ensureDataDir();
 const {running,pid}=bridgeAlive(dir);
 const cfg=readJson(path.join(dir,'bridge.json'));
 const store=cfg?.local_store&&fs.existsSync(cfg.local_store)?cfg.local_store:null;
 const observed=file=>store?localData.read(path.join(store,file))?.observed_at||null:null;
 const bridgeLog=path.join(dir,path.basename(binPath('bridge'))+'.log');
 const bridgeErr=path.join(dir,path.basename(binPath('bridge'))+'-error.log');
 return {
  running,paused:desktopBridge.isPaused(dir),pid,
  binary_present:fs.existsSync(binPath('bridge')),
  configured:!!(cfg&&cfg.server&&readSecret('bridge-token')),
  server:cfg?.server||null,
  interval_seconds:cfg?.interval_seconds||null,
  codex_observed_at:observed('codex-latest.json'),
  cursor_observed_at:observed('cursor-latest.json'),
  log_tail:desktopBridge.lastLogLine(bridgeLog)||desktopBridge.lastLogLine(bridgeErr),
  last_error:lastError||null,
 };
}
function pauseBridge(){
 desktopBridge.setPaused(ensureDataDir(),true);
 stopBridge();
 return bridgeStatus();
}
async function resumeBridge(){
 desktopBridge.setPaused(ensureDataDir(),false);
 await ensureBridge(ensureDataDir(),loadConfig(ensureDataDir()));
 return bridgeStatus();
}
function scanRoots(){
 return {roots:desktopBridge.readScanRoots(ensureDataDir())};
}
function addScanRoots(inputs){
 const dir=ensureDataDir();
 const added=desktopBridge.normalizeRoots(inputs);
 for(const root of added){
  let stat=null;try{stat=fs.statSync(root);}catch{}
  if(!stat||!stat.isDirectory())throw Error('授权目录不存在或不是文件夹：'+root);
 }
 return desktopBridge.addScanRoots(dir,added);
}
function removeScanRoot(root){return desktopBridge.removeScanRoot(ensureDataDir(),root);}
// The desktop tool runs the bundled bridge binary as a bounded stdin/stdout
// parser: no network, no credentials, no arbitrary arguments.
function runDesktopTool(input){
 const exe=binPath('bridge');
 if(!fs.existsSync(exe))return Promise.reject(new Error('缺少内置桌面工具程序，请重新安装'));
 return new Promise((resolve,reject)=>{
  const child=spawn(exe,['--desktop-tool'],{cwd:path.dirname(exe),env:{...process.env},stdio:['pipe','pipe','pipe'],windowsHide:true});
  let out='',err='',done=false;
  const finish=fn=>{if(done)return;done=true;clearTimeout(timer);fn();};
  const timer=setTimeout(()=>finish(()=>{child.kill();reject(new Error('桌面工具超时'));}),60000);
  let outputBytes=0;
  child.stdout.setEncoding('utf8');
  child.stdout.on('data',d=>{outputBytes+=Buffer.byteLength(d);if(outputBytes>48*1024*1024){finish(()=>{child.kill();reject(new Error('桌面工具输出超过限制'));});return;}out+=d;});
  child.stderr.on('data',d=>{err=(err+d).slice(-65536);});
  child.on('error',e=>finish(()=>reject(new Error('桌面工具无法启动：'+e.message))));
  child.on('close',code=>finish(()=>{
   if(code!==0)return reject(new Error((err.trim().split(/\r?\n/).pop()||'桌面工具运行失败').slice(0,300)));
   try{resolve(JSON.parse(out));}catch{reject(new Error('桌面工具输出无效'));}
  }));
  child.stdin.on('error',()=>{});
  child.stdin.end(JSON.stringify(input));
 });
}
function runScanRoots(){
 const roots=desktopBridge.readScanRoots(ensureDataDir());
 if(!roots.length)throw Error('请先添加授权目录');
 if(!roots.every(root=>fs.existsSync(root)))throw Error('授权目录已不存在，请重新选择');
 return runDesktopTool({action:'scan',roots});
}
async function diagnostics(){
 const dir=ensureDataDir(),config=loadConfig(dir);
 const cloud=await probeCloud(getUiOrigin()).catch(()=>false);
 return {
  generated_at:new Date().toISOString(),
  version:app.getVersion(),
  platform:process.platform,
  electron:process.versions.electron,
  mode:getActiveMode(config),
  ui_origin:getUiOrigin(),
  cloud_ready:cloud,
  data_dir:dir,
  bridge:bridgeStatus(),
  scan_roots:desktopBridge.readScanRoots(dir),
  startup_message:startupMessage()||null,
 };
}
module.exports.localRequest=localRequest;
module.exports.localStatus=localStatus;
module.exports.syncLocalData=syncLocalData;
module.exports={refreshSessionForSync,localRequest,localStatus,syncLocalData,ensureReady,stopManaged,stopBridge,startupMessage,getUiOrigin,getActiveMode,setUserMode,setCloudServer,installationID,extensionPairCode,forceLocalMode,resetMode,clearModePreference,readModePreference,probeCloud,loadConfig,dataDir,readDesktopSession,writeDesktopSession,syncBridgeFromSession,bridgeStatus,pauseBridge,resumeBridge,scanRoots,addScanRoots,removeScanRoot,runScanRoots,runDesktopTool,diagnostics,LOCAL_ORIGIN,ORIGIN:LOCAL_ORIGIN,DEFAULT_CLOUD};
