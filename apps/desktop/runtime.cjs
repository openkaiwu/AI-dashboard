const {app,safeStorage}=require('electron');
const {spawn}=require('node:child_process');
const crypto=require('node:crypto');
const fs=require('node:fs');
const https=require('node:https');
const path=require('node:path');
const platform=require('./platform.cjs');

const LOCAL_ORIGIN='http://127.0.0.1:8080';
const DEFAULT_CLOUD='http://127.0.0.1:8080';
let lastError='';
let managed={server:null,bridge:null};
let cachedConfig=null;
let activeMode=null;

function readJson(file){try{return JSON.parse(fs.readFileSync(file,'utf8'));}catch{return null;}}
function dataDir(){return path.join(app.getPath('userData'),'runtime');}
function ensureDataDir(){const dir=dataDir();fs.mkdirSync(dir,{recursive:true});return dir;}
function secretPath(name){return path.join(ensureDataDir(),name+'.protected');}
function readSecret(name){try{return safeStorage.decryptString(fs.readFileSync(secretPath(name)));}catch{return '';}}
function writeSecret(name,value){if(!safeStorage.isEncryptionAvailable())throw Error('系统凭据保护不可用');fs.writeFileSync(secretPath(name),safeStorage.encryptString(value),{mode:0o600});}
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
 const cfg={uiMode:'ask',cloudServer:DEFAULT_CLOUD,profileId:'public-cloud',profileName:'公网 AI Hub',bridgeIntervalSeconds:300,...bundledConfig(),...readJson(path.join(dir,'app-config.json'))};
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
   res.on('end',()=>{try{const parsed=JSON.parse(raw||'{}');if(res.statusCode>=300)reject(new Error(parsed.message||parsed.error||`HTTP ${res.statusCode}`));else resolve(parsed);}catch(e){reject(e);}});
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
 if(!res.ok)throw new Error(parsed.message||parsed.error||`请求失败 (${res.status})`);
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
  const seed=readJson(example)||{server:config.cloudServer||DEFAULT_CLOUD,codex_path:'',cursor_enabled:true,interval_seconds:300};
 delete seed.token;
 fs.writeFileSync(cfgPath,JSON.stringify(seed,null,2));
 return cfgPath;
}
async function ensureBridge(dir,config){
 ensureBridgeConfig(dir,config);
 const cfgPath=path.join(dir,'bridge.json');
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
 if(fs.existsSync(stateFile)){
  try{const saved=JSON.parse(fs.readFileSync(stateFile,'utf8'));process.kill(saved.Id,0);return;}catch{fs.unlinkSync(stateFile);}
 }
 const child=spawnHidden(bridgeExe,['--config',cfgPath],dir,{AIHUB_BRIDGE_TOKEN:readSecret('bridge-token'),AIHUB_EXTENSION_PAIR_CODE:extensionPairCode()});
 fs.writeFileSync(stateFile,JSON.stringify({Id:child.pid,Binary:bridgeExe}));
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
   await ensurePostgres(dir);
   await ensureServer(dir);
  }
  ensureBridgeConfig(dir,config);
 }catch(e){
  lastError=e.message||String(e);
  throw e;
 }
}
async function syncBridgeFromSession(webContents,config){
 const dir=ensureDataDir();
 const picked=await webContents.executeJavaScript(`(()=>{try{const id=localStorage.getItem('hub_profile');const profiles=JSON.parse(localStorage.getItem('hub_profiles')||'[]');const profile=profiles.find(p=>p.id===id)||profiles[0]||{id:'default',url:location.origin};return {id:profile.id,server:profile.url};}catch{return null;}})()`,true);
 const session=picked?readDesktopSession(picked.id):null;
 if(!session?.token){stopBridge();return;}
 const existing=readJson(path.join(dir,'bridge.json'));
 if(bridgeReady(existing)&&existing.server===picked.server&&existing.device_id===session.device_id){await ensureBridge(dir,config);return;}
 stopBridge();
 try{
  const token=await createBridgeToken(picked.server,session.token,'我的电脑');
  writeBridge(dir,{server:picked.server,device_id:session.device_id,codex_path:await platform.findCodex()||'',cursor_enabled:true,interval_seconds:config.bridgeIntervalSeconds||300},token);
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
module.exports={ensureReady,stopManaged,stopBridge,startupMessage,getUiOrigin,getActiveMode,setUserMode,setCloudServer,extensionPairCode,forceLocalMode,resetMode,clearModePreference,readModePreference,probeCloud,loadConfig,dataDir,readDesktopSession,writeDesktopSession,syncBridgeFromSession,LOCAL_ORIGIN,ORIGIN:LOCAL_ORIGIN,DEFAULT_CLOUD};
