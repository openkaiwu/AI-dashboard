const {app,BrowserWindow,Tray,Menu,nativeImage,ipcMain,dialog,shell}=require('electron');
const fs=require('node:fs');
const path=require('node:path');
const runtime=require('./runtime.cjs');
let win,tray,quitting=false,alertOpen=false;
app.commandLine.appendSwitch('ignore-certificate-errors');
function uiOrigin(){return runtime.getUiOrigin();}
function allowOrigin(url){try{return new URL(url).origin===uiOrigin();}catch{return false;}}
if(!app.requestSingleInstanceLock())app.quit();
else {
 app.on('second-instance',()=>{win?.show();win?.focus();});
 app.on('before-quit',()=>{quitting=true;runtime.stopManaged();});
 app.whenReady().then(async()=>{
  app.setAppUserModelId('local.aihub.codex');
  win=new BrowserWindow({width:1220,height:860,minWidth:420,minHeight:600,title:'AI Hub · Codex 使用助手',backgroundColor:'#ffffff',show:false,webPreferences:{preload:path.join(__dirname,'preload.cjs'),nodeIntegration:false,contextIsolation:true,sandbox:true,backgroundThrottling:false}});
  win.removeMenu();
  win.webContents.on('before-input-event',(event,input)=>{if(input.control&&input.shift&&input.key.toLowerCase()==='t'&&input.type==='keyDown'){event.preventDefault();void showAlert({title:'强提醒测试',body:'这是演示，不会改变真实额度或提醒状态。'});}});
  win.on('close',e=>{if(!quitting){e.preventDefault();win.hide();}});
  win.webContents.setWindowOpenHandler(({url})=>{if(/^https:\/\/(x\.com|developers\.openai\.com|learn\.chatgpt\.com)\//.test(url))void shell.openExternal(url);return {action:'deny'};});
  win.webContents.on('will-navigate',(e,url)=>{if(!allowOrigin(url))e.preventDefault();});
  win.webContents.session.setPermissionRequestHandler((_wc,_permission,callback)=>callback(false));
  const icon=nativeImage.createFromPath(path.join(__dirname,'icon.png'));win.setIcon(icon);
  tray=new Tray(icon);tray.setToolTip('AI Hub · 额度强提醒');
  function buildTrayMenu(){
   tray.setContextMenu(Menu.buildFromTemplate([
    {label:'打开 AI Hub',click:()=>{win.show();win.focus();}},
    {label:'切换使用方式',click:()=>void switchMode()},
    {label:'重新连接',click:()=>void reconnect(false)},
    {type:'separator'},
    {label:'退出',click:()=>app.quit()},
   ]));
  }
  buildTrayMenu();
  tray.on('double-click',()=>{win.show();win.focus();});
  ipcMain.handle('quota-alert',async(e,value)=>{
   if(e.sender!==win.webContents||e.senderFrame!==win.webContents.mainFrame||!allowOrigin(e.senderFrame.url))throw Error('Unauthorized sender');
   return showAlert(value);
  });
  async function showAlert(value){
   if(alertOpen||!value||typeof value.title!=='string'||typeof value.body!=='string'||value.title.length>200||value.body.length>3000)return 'retry';
   alertOpen=true;win.show();win.restore();win.focus();win.flashFrame(true);
   try{const result=await dialog.showMessageBox(win,{type:'warning',title:'AI Hub · 额度强提醒',message:value.title,detail:value.body+'\n\n当前采集为账户额度，无法定位具体任务的 token 消耗。',buttons:['已读','1 小时后提醒','忽略本次','打开 Codex 手动停止'],defaultId:0,cancelId:0,noLink:true});
    if(result.response===3){await shell.openExternal('codex://');return 'read';}
    return ['read','snooze','dismiss'][result.response]||'read';
   }finally{win.flashFrame(false);alertOpen=false;}
  }
  ipcMain.handle('open-codex',async e=>{if(e.sender!==win.webContents||e.senderFrame!==win.webContents.mainFrame||!allowOrigin(e.senderFrame.url))throw Error('Unauthorized sender');await shell.openExternal('codex://');});
  async function askMode(force=false){
   const pref=runtime.readModePreference();
   if(!force&&pref?.remember&&pref.mode){runtime.setUserMode(pref.mode);return pref.mode;}
   win.show();win.focus();
   const pick=await dialog.showMessageBox(win,{type:'question',title:'AI Hub · 选择使用方式',message:'你想如何开始使用？',detail:`【登录服务器】连接 ${runtime.DEFAULT_CLOUD}，使用账户登录并同步 Codex 数据。\n\n【离线本地】仅在本机运行（127.0.0.1），查看演示数据，需 WSL + PostgreSQL。`,buttons:['登录服务器','离线本地模式'],defaultId:0,cancelId:1,noLink:true});
   const mode=pick.response===0?'cloud':'local';
   const remember=await dialog.showMessageBox(win,{type:'question',title:'记住选择？',message:'下次启动是否直接使用该模式？',buttons:['记住','每次询问'],defaultId:0,cancelId:1,noLink:true});
   fs.writeFileSync(path.join(runtime.dataDir(),'mode-preference.json'),JSON.stringify({mode,remember:remember.response===0},null,2));
   runtime.setUserMode(mode);
   return mode;
  }
  async function openApp(){
   const config=runtime.loadConfig(runtime.dataDir());
   const origin=uiOrigin();
   await win.loadURL(origin+'/login');
   await runtime.injectWebSession(win.webContents,config);
   await runtime.syncBridgeFromSession(win.webContents,config);
   await win.loadURL(origin+'/');
   win.show();
  }
  async function handleCloudFailure(err){
   const hint=runtime.startupMessage()||err?.message||String(err||'连接失败');
   const pick=await dialog.showMessageBox(win,{type:'warning',title:'无法连接服务器',message:hint,detail:'可能原因：网络代理（Clash 等）、IP 证书、或服务器 443 未放行。可改选离线本地模式继续使用。',buttons:['重试服务器','改用离线本地','退出'],defaultId:1,cancelId:2,noLink:true});
   if(pick.response===2)return app.quit();
   if(pick.response===1){runtime.setUserMode('local');await reconnect(true);return;}
   await reconnect(false);
  }
  async function reconnect(skipAsk){
   runtime.resetMode();
   runtime.stopManaged();
   await start(skipAsk);
  }
  async function switchMode(){
   runtime.clearModePreference();
   runtime.stopManaged();
   await start(true);
  }
  async function start(skipAsk=false){
   try{
    await askMode(skipAsk);
    const mode=runtime.getActiveMode();
    if(mode==='cloud'&&!await runtime.probeCloud(runtime.loadConfig(runtime.dataDir()).cloudServer)){
     return handleCloudFailure(new Error('公网服务器不可达'));
    }
    await runtime.ensureReady();
    await openApp();
   }catch(err){
    if(runtime.getActiveMode()==='cloud')return handleCloudFailure(err);
    await dialog.showMessageBox(win,{type:'error',title:'离线模式启动失败',message:runtime.startupMessage()||err.message||String(err),detail:'请确认已安装 WSL 与 PostgreSQL 14。'});
    win.show();
   }
  }
  win.webContents.on('did-finish-load',()=>{void runtime.syncBridgeFromSession(win.webContents,runtime.loadConfig(runtime.dataDir()));});
  await start(false);
 });
}
