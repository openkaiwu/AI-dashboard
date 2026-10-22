const {test}=require('node:test');
const assert=require('node:assert/strict');
const vm=require('node:vm');
const fs=require('node:fs');
const {EventEmitter}=require('node:events');
test('desktop quota entry stays silent even when invoked by an old renderer',async()=>{
 const handlers={};let window,answer=1,openCount=0;
 class Window extends EventEmitter {
  constructor(){super();window=this;this.webContents=new EventEmitter();Object.assign(this.webContents,{mainFrame:{url:'http://127.0.0.1:8080/codex'},setWindowOpenHandler(){},session:{setPermissionRequestHandler(){}}});}
  removeMenu(){} setIcon(){} show(){} restore(){} focus(){} flashFrame(){} async loadURL(){}
 }
 const {loginItemSupportedFor}=require('./platform.cjs');
 const loginItemCalls=[];
 const runtime={ensureReady:async()=>{},stopManaged(){},startupMessage(){return ''},getUiOrigin:()=>'http://127.0.0.1:8080',getActiveMode:()=>'local',setUserMode(){},forceLocalMode(){},resetMode(){},clearModePreference(){},readModePreference:()=>null,probeCloud:async()=>true,loadConfig:()=>({uiMode:'local',profileId:'local-default',profileName:'本地'}),dataDir:()=>'',injectWebSession:async()=>false,syncBridgeFromSession:async()=>{},DEFAULT_CLOUD:'https://hub.example.com',ORIGIN:'http://127.0.0.1:8080'};
 const electron={app:{requestSingleInstanceLock:()=>true,on(){},whenReady:()=>Promise.resolve(),setAppUserModelId(){},setLoginItemSettings(settings){loginItemCalls.push(settings);},commandLine:{appendSwitch(){}}},BrowserWindow:Window,Tray:class{setToolTip(){}setContextMenu(){}on(){}},Menu:{buildFromTemplate:x=>x},nativeImage:{createFromPath:()=>({resize:()=>({})})},ipcMain:{handle:(k,v)=>handlers[k]=v,on:(k,v)=>handlers[k]=v},dialog:{showMessageBox:async()=>{openCount++;return {response:answer};}},shell:{openExternal:async url=>{assert.equal(url,'codex://');}}};
 vm.runInNewContext(fs.readFileSync(__dirname+'/main.cjs','utf8'),{require:name=>name==='electron'?electron:name==='./runtime.cjs'?runtime:require(name),__dirname,URL,setInterval:()=>({unref(){}}),setImmediate});
 await new Promise(r=>setImmediate(r));
 if(loginItemSupportedFor(process.platform)){
  assert.equal(loginItemCalls.length,1);
  assert.equal(loginItemCalls[0].openAtLogin,true);
  assert.equal(loginItemCalls[0].args.length,1);
  assert.equal(loginItemCalls[0].args[0],'--startup');
 }else{
  assert.equal(loginItemCalls.length,0);
 }
 openCount=0;
 const e={sender:window.webContents,senderFrame:window.webContents.mainFrame};
 await assert.rejects(handlers['quota-alert']({...e,sender:{}},{title:'x',body:'y'}));
 assert.equal(openCount,0);
 for(const value of [{title:'quota',body:'warning'},{title:'quota',body:'warning',provider:'Codex'},{title:'x'.repeat(201),body:'warning'}]) {
  assert.equal(await handlers['quota-alert'](e,value),'silent');
 }
 assert.equal(openCount,0);
 await assert.rejects(handlers['open-codex']({...e,senderFrame:{url:'https://untrusted.example'}}));
});
