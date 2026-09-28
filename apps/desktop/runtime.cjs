const {app}=require('electron');
const {spawn,execFile}=require('node:child_process');
const crypto=require('node:crypto');
const fs=require('node:fs');
const https=require('node:https');
const path=require('node:path');
const {promisify}=require('node:util');
const execFileAsync=promisify(execFile);

const LOCAL_ORIGIN='http://127.0.0.1:8080';
const DEFAULT_CLOUD='https://hub.example.com';
let lastError='';
let managed={server:null,bridge:null};
let cachedConfig=null;
let activeMode=null;

function readJson(file){try{return JSON.parse(fs.readFileSync(file,'utf8'));}catch{return null;}}
function dataDir(){return path.join(app.getPath('userData'),'runtime');}
function ensureDataDir(){const dir=dataDir();fs.mkdirSync(dir,{recursive:true});return dir;}
const DEV_BIN={'aihub-server.exe':'aihub-windows-amd64.exe','aihub-bridge.exe':'aihub-bridge-windows-amd64.exe'};
function binPath(name){
 if(!app.isPackaged){return path.join(__dirname,'..','..','artifacts','aihub-m0',DEV_BIN[name]||name);}
 return path.join(process.resourcesPath,'bin',name);
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
function migrateFromDev(dir){
 if(fs.existsSync(path.join(dir,'bridge.json')))return;
 const roots=[process.env.AIHUB_DEV_ROOT,path.join(process.env.USERPROFILE||'','source','repos','AIDASH')].filter(Boolean);
 for(const root of roots){
  const src=path.join(root,'.runtime');
  if(!fs.existsSync(src))continue;
  for(const name of ['bridge.json','bootstrap.json','server.env']){
   const from=path.join(src,name),to=path.join(dir,name);
   if(fs.existsSync(from)&&!fs.existsSync(to))fs.copyFileSync(from,to);
  }
  return;
 }
}
function readEnvFile(file){
 const line=fs.readFileSync(file,'utf8').split(/\r?\n/).find(l=>l.startsWith('export AIHUB_DATABASE_URL='));
 if(!line)throw new Error('server.env 缺少数据库配置');
 return line.slice('export AIHUB_DATABASE_URL='.length).trim().replace(/^"|"$/g,'');
}
async function execWsl(script){
 await execFileAsync('wsl.exe',['-e','bash','-lc',script],{windowsHide:true});
}
async function ensureWsl(){
 try{await execFileAsync('wsl.exe',['-e','true'],{windowsHide:true});return true;}catch{throw new Error('未检测到 WSL，请先安装 WSL 与 PostgreSQL');}
}
async function ensurePostgres(dir){
 await ensureWsl();
 const keepFile=path.join(dir,'wsl-keepalive.json');
 let alive=false;
 if(fs.existsSync(keepFile)){
  try{const saved=JSON.parse(fs.readFileSync(keepFile,'utf8'));process.kill(saved.Id,0);alive=true;}catch{}
 }
 if(!alive){
  const keep=spawn('wsl.exe',['-e','sleep','infinity'],{stdio:'ignore',windowsHide:true,detached:true});
  keep.unref();
  fs.writeFileSync(keepFile,JSON.stringify({Id:keep.pid,StartTicks:String(keep.spawnfile||'')}));
 }
 await execWsl('for v in 14 16 15 13; do pg_ctlcluster "$v" main status >/dev/null 2>&1 && { pg_ctlcluster "$v" main start; exit 0; }; done; pg_ctlcluster 14 main start');
}
async function ensureServerEnv(dir){
 const envFile=path.join(dir,'server.env');
 if(fs.existsSync(envFile))return readEnvFile(envFile);
 const password=crypto.randomBytes(24).toString('hex');
 const sql=`DO \\$\\$ BEGIN CREATE ROLE aihub_m0 LOGIN PASSWORD '${password}'; EXCEPTION WHEN duplicate_object THEN NULL; END \\$\\$; DO \\$\\$ BEGIN CREATE DATABASE aihub_m0 OWNER aihub_m0; EXCEPTION WHEN duplicate_database THEN NULL; END \\$\\$;`;
 await execWsl(`(command -v runuser >/dev/null && runuser -u postgres -- psql -v ON_ERROR_STOP=1 -c "${sql}") || sudo -u postgres psql -v ON_ERROR_STOP=1 -c "${sql}"`);
 const url=`postgres://aihub_m0:${password}@127.0.0.1:5432/aihub_m0?sslmode=disable`;
 fs.writeFileSync(envFile,`export AIHUB_DATABASE_URL="${url}"\n`,{mode:0o600});
 return url;
}
function httpsJson(method,urlStr,body,headers={}){
 return new Promise((resolve,reject)=>{
  const u=new URL(urlStr);
  const data=body?JSON.stringify(body):null;
  const req=https.request({hostname:u.hostname,port:u.port||443,path:u.pathname+u.search,method,headers:{'Content-Type':'application/json',...headers,...(data?{'Content-Length':Buffer.byteLength(data)}:{})},rejectUnauthorized:false,timeout:15000,servername:u.hostname},res=>{
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
 const serverExe=binPath('aihub-server.exe');
 if(!fs.existsSync(serverExe))throw new Error('缺少内置服务端程序');
 const dbUrl=await ensureServerEnv(dir);
 const pidFile=path.join(dir,'server.pid');
 if(fs.existsSync(pidFile)){
  try{process.kill(Number(fs.readFileSync(pidFile,'utf8').trim()),0);if(await waitReady(LOCAL_ORIGIN,3000))return;}catch{}
 }
 const child=spawnHidden(serverExe,[],dir,{AIHUB_DATABASE_URL:dbUrl,AIHUB_ADDR:'127.0.0.1:8080',AIHUB_DEMO:'true'});
 fs.writeFileSync(pidFile,String(child.pid));
 managed.server=child;
 child.on('exit',()=>{if(managed.server===child)managed.server=null;});
 if(!await waitReady(LOCAL_ORIGIN,45000))throw new Error('本地服务启动超时，请确认 WSL 已安装且 PostgreSQL 可用');
}
function findCodexExe(){
 const roots=[path.join(process.env.LOCALAPPDATA||'','OpenAI','Codex','bin')];
 for(const root of roots){
  if(!fs.existsSync(root))continue;
  const stack=[root];
  while(stack.length){
   const dir=stack.pop();
   for(const entry of fs.readdirSync(dir,{withFileTypes:true})){
    const full=path.join(dir,entry.name);
    if(entry.isDirectory())stack.push(full);
    else if(entry.name==='codex.exe')return full;
   }
  }
 }
 return '';
}
function bridgeReady(cfg){
 return cfg&&cfg.token&&cfg.token!=='CREATE_A_CONNECTION_IN_AI_HUB'&&cfg.server;
}
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
async function cloudLogin(server,email,password,deviceName){
 return apiJson(`${server.replace(/\/$/,'')}/api/v1/auth/login`,{method:'POST',body:JSON.stringify({email,password,device_name:deviceName})});
}
async function createBridgeToken(server,accessToken,name){
 const out=await apiJson(`${server.replace(/\/$/,'')}/api/v1/codex/bridges`,{method:'POST',headers:{Authorization:`Bearer ${accessToken}`},body:JSON.stringify({name})});
 return out.token;
}
function writeBridge(dir,cfg){fs.writeFileSync(path.join(dir,'bridge.json'),JSON.stringify(cfg,null,2));}
async function autoProvisionBridge(dir,config){
 const cfgPath=path.join(dir,'bridge.json');
 let cfg=readJson(cfgPath);
 if(bridgeReady(cfg)){
  const codex=cfg.codex_path&&fs.existsSync(cfg.codex_path)?cfg.codex_path:findCodexExe();
  if(codex&&cfg.codex_path!==codex){cfg.codex_path=codex;writeBridge(dir,cfg);}
  return cfg;
 }
 const bootstrap=readJson(path.join(dir,'bootstrap.json'));
 if(!bootstrap?.email||!bootstrap?.password)return cfg;
 const server=(bootstrap.cloudServer||config.cloudServer||DEFAULT_CLOUD).replace(/\/$/,'');
 try{
  const session=await cloudLogin(server,bootstrap.email,bootstrap.password,bootstrap.deviceName||'我的电脑');
  const token=await createBridgeToken(server,session.token,bootstrap.bridgeName||'我的电脑');
  cfg={server,token,codex_path:findCodexExe()||'',cursor_enabled:true,interval_seconds:config.bridgeIntervalSeconds||300};
  writeBridge(dir,cfg);
  return cfg;
 }catch(e){
  lastError=`云端采集器配置失败：${e.message}`;
  return cfg;
 }
}
function ensureBridgeConfig(dir,config){
 const cfgPath=path.join(dir,'bridge.json');
 if(fs.existsSync(cfgPath))return cfgPath;
 const example=app.isPackaged?path.join(process.resourcesPath,'bridge.example.json'):path.join(__dirname,'resources','bridge.example.json');
  const seed=readJson(example)||{server:config.cloudServer||DEFAULT_CLOUD,token:'CREATE_A_CONNECTION_IN_AI_HUB',codex_path:'',cursor_enabled:true,interval_seconds:300};
 fs.writeFileSync(cfgPath,JSON.stringify(seed,null,2));
 return cfgPath;
}
async function ensureBridge(dir,config){
 ensureBridgeConfig(dir,config);
 await autoProvisionBridge(dir,config);
 const cfgPath=path.join(dir,'bridge.json');
 let cfg=readJson(cfgPath);
 if(!bridgeReady(cfg))return;
 if(!cfg.codex_path||!fs.existsSync(cfg.codex_path)){
  const found=findCodexExe();
  if(!found)return;
  cfg.codex_path=found;
  writeBridge(dir,cfg);
 }
 const bridgeExe=binPath('aihub-bridge.exe');
 if(!fs.existsSync(bridgeExe))return;
 const stateFile=path.join(dir,'bridge-process.json');
 if(fs.existsSync(stateFile)){
  try{const saved=JSON.parse(fs.readFileSync(stateFile,'utf8'));process.kill(saved.Id,0);return;}catch{fs.unlinkSync(stateFile);}
 }
 const child=spawnHidden(bridgeExe,['--config',cfgPath],dir);
 fs.writeFileSync(stateFile,JSON.stringify({Id:child.pid,Binary:bridgeExe}));
 managed.bridge=child;
 child.on('exit',(code)=>{if(managed.bridge===child){managed.bridge=null;try{fs.unlinkSync(stateFile);}catch{}} if(code)lastError=`Codex 采集器退出 (${code})`;});
}
async function ensureReady(){
 if(process.env.AIHUB_DESKTOP_SKIP_RUNTIME==='1')return;
 try{
  const dir=ensureDataDir();
  migrateFromDev(dir);
  const config=loadConfig(dir);
  await resolveActiveMode(dir,config);
  if(getActiveMode(config)==='local'){
   await ensurePostgres(dir);
   await ensureServer(dir);
  }
  await ensureBridge(dir,config);
 }catch(e){
  lastError=e.message||String(e);
  throw e;
 }
}
function readBootstrap(dir=ensureDataDir()){return readJson(path.join(dir,'bootstrap.json'));}
async function injectWebSession(webContents,config){
 const mode=getActiveMode(config);
 const bootstrap=readBootstrap();
 const has=await webContents.executeJavaScript(`(()=>{try{const id=localStorage.getItem('hub_profile')||'${mode==='local'?'local-default':config.profileId}';return !!localStorage.getItem('hub_session:'+id);}catch{return false;}})()`,true);
 if(has)return true;
 let session;
 let profile;
 if(mode==='local'){
  session=await apiJson(`${LOCAL_ORIGIN}/api/v1/auth/login`,{method:'POST',body:JSON.stringify({email:'demo@aihub.local',password:'demo1234',device_name:'我的电脑'})});
  profile={id:'local-default',name:'本地 AI Hub',url:LOCAL_ORIGIN};
 }else{
  if(!bootstrap?.email||!bootstrap?.password)return false;
  const server=(bootstrap.cloudServer||config.cloudServer||DEFAULT_CLOUD).replace(/\/$/,'');
  session=await cloudLogin(server,bootstrap.email,bootstrap.password,bootstrap.deviceName||'我的电脑');
  profile={id:config.profileId,name:config.profileName,url:server};
 }
 await webContents.executeJavaScript(`(()=>{const profile=${JSON.stringify(profile)};const session=${JSON.stringify(session)};localStorage.setItem('hub_profiles',JSON.stringify([profile]));localStorage.setItem('hub_profile',profile.id);localStorage.setItem('hub_session:'+profile.id,JSON.stringify(session));localStorage.setItem('hub_advisor:'+profile.url+'|'+session.user.id+':strong','on');})()`,true);
 return true;
}
async function syncBridgeFromSession(webContents,config){
 if(getActiveMode(config)==='local')return;
 const dir=ensureDataDir();
 if(bridgeReady(readJson(path.join(dir,'bridge.json'))))return;
 const picked=await webContents.executeJavaScript(`(()=>{try{const id=localStorage.getItem('hub_profile');const profiles=JSON.parse(localStorage.getItem('hub_profiles')||'[]');const profile=profiles.find(p=>p.id===id)||profiles[0];const session=JSON.parse(localStorage.getItem('hub_session:'+profile.id)||'null');return session?{server:profile.url,token:session.token}:null;}catch{return null;}})()`,true);
 if(!picked?.token)return;
 try{
  const token=await createBridgeToken(picked.server,picked.token,'我的电脑');
  writeBridge(dir,{server:picked.server,token,codex_path:findCodexExe()||'',interval_seconds:config.bridgeIntervalSeconds||300});
  await ensureBridge(dir,config);
 }catch{}
}
function stopManaged(){
 for(const key of ['bridge','server']){
  const child=managed[key];
  if(child&&!child.killed){try{child.kill();}catch{}}
  managed[key]=null;
 }
}
function startupMessage(){return lastError||'';}
module.exports={ensureReady,stopManaged,startupMessage,getUiOrigin,getActiveMode,setUserMode,forceLocalMode,resetMode,clearModePreference,readModePreference,probeCloud,loadConfig,dataDir,injectWebSession,syncBridgeFromSession,LOCAL_ORIGIN,ORIGIN:LOCAL_ORIGIN,DEFAULT_CLOUD};
