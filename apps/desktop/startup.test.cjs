const {test}=require('node:test');
const assert=require('node:assert/strict');
const vm=require('node:vm');
const fs=require('node:fs');
const {EventEmitter}=require('node:events');

function harness({failures=0,remember=true,startup=false}={}){
 const calls={load:0,dialogs:0,sync:0,show:0,urls:[],delays:[],mode:'cloud'};
 let window;const menu=[];
 class Window extends EventEmitter{
  constructor(){super();window=this;this.webContents=new EventEmitter();Object.assign(this.webContents,{mainFrame:{url:'https://hub.example.com/'},setWindowOpenHandler(){},session:{setPermissionRequestHandler(){}}});}
  removeMenu(){} setIcon(){} show(){calls.show++;} focus(){} async loadFile(){}
  async loadURL(url){calls.urls.push(url);calls.load++;if(calls.load<=failures)throw Error('ERR_INTERNET_DISCONNECTED');this.webContents.emit('did-finish-load');}
 }
 const runtime={getUiOrigin:()=> 'https://hub.example.com',getActiveMode:()=>calls.mode,readModePreference:()=>({remember,mode:'cloud'}),setUserMode:m=>calls.mode=m,loadConfig:()=>({cloudServer:'https://hub.example.com'}),DEFAULT_CLOUD:'http://127.0.0.1:8080',ensureReady:async()=>{},syncBridgeFromSession:async()=>{calls.sync++;},stopManaged(){},resetMode(){},clearModePreference(){remember=false;},dataDir:()=>'',startupMessage:()=>''};
 const electron={app:{requestSingleInstanceLock:()=>true,on(){},whenReady:()=>Promise.resolve(),setAppUserModelId(){},setLoginItemSettings(){},quit(){}},BrowserWindow:Window,Tray:class{setToolTip(){}setContextMenu(items){menu.splice(0,menu.length,...items);}on(){}},Menu:{buildFromTemplate:x=>x},nativeImage:{createFromPath:()=>({})},ipcMain:{handle(){},on(){}},dialog:{showMessageBox:async()=>{calls.dialogs++;return {response:0};}},shell:{}};
 const fakeFs={...fs,writeFileSync(){}};
 vm.runInNewContext(fs.readFileSync(__dirname+'/main.cjs','utf8'),{require:n=>n==='electron'?electron:n==='./runtime.cjs'?runtime:n==='node:fs'?fakeFs:require(n),__dirname,URL,process:{argv:startup?['AIHub','--startup']:['AIHub'],env:{}},setInterval:()=>({unref(){}}),setTimeout:(fn,ms)=>{calls.delays.push(ms);setImmediate(fn);},setImmediate});
 return {calls,menu,get window(){return window;}};
}
const settle=async()=>{for(let i=0;i<15;i++)await new Promise(r=>setImmediate(r));};
test('Windows startup flag opens the quota window and restores the remembered connection',async()=>{
 const h=harness({startup:true});await settle();
 assert.equal(h.calls.dialogs,0);assert.equal(h.calls.load,1);assert.ok(h.calls.show>0);assert.ok(h.calls.sync>0);
 assert.deepEqual(h.calls.urls,['https://hub.example.com/codex']);
});
test('remembered cloud startup recovers when network is late without mode or failure dialogs',async()=>{
 const h=harness({failures:3});await settle();
 assert.equal(h.calls.load,4);assert.equal(h.calls.dialogs,0);assert.ok(h.calls.sync>0);
 assert.deepEqual(h.calls.delays,[1000,2000,4000]);
});
test('tray reconnect preserves server mode and never asks the remembered selection again',async()=>{
 const h=harness();await settle();await h.menu.find(x=>x.label==='重新连接').click();await settle();
 assert.equal(h.calls.load,2);assert.equal(h.calls.dialogs,0);
});
test('explicit switch still asks the user for mode',async()=>{
 const h=harness();await settle();await h.menu.find(x=>x.label==='切换使用方式').click();await settle();
 assert.equal(h.calls.dialogs,2);assert.equal(h.calls.load,2);
});
