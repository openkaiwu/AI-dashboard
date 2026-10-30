// Real UI, preload and read-only native state. Switch uses a cancellation fixture.
const {app,BrowserWindow,ipcMain}=require('electron');
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),assert=require('node:assert/strict');
const {execFile}=require('node:child_process');
app.setPath('userData',fs.mkdtempSync(path.join(os.tmpdir(),'aihub-timezone-')));
const resources=process.argv[2]?path.resolve(process.argv[2],'resources'):path.resolve(__dirname,'../apps/desktop/resources');
const desktop=process.argv[2]?path.join(resources,'app.asar'):path.resolve(__dirname,'../apps/desktop');
setTimeout(()=>{console.error('Timezone integration timed out');app.exit(1);},25000);
app.whenReady().then(async()=>{
 const win=new BrowserWindow({show:false,width:1280,height:860,webPreferences:{preload:path.join(desktop,'preload.cjs'),sandbox:true,contextIsolation:true,nodeIntegration:false}});
 const origin='https://aihub.example.invalid';let applies=0;
 ipcMain.on('desktop-info',e=>{e.returnValue={platform:'win32',version:'test',bundledUI:true};});
 ipcMain.on('session-get',e=>{e.returnValue={blocked:true,token:'fixture',user:{id:'fixture',email:'fixture@example.invalid'}};});
 const controller=require(path.join(desktop,'timeZoneTool.cjs')).createController({root:resources,execute:(file,args,options,callback)=>{
  if(args[0]==='--switch'){applies++;callback(Error('cancel fixture'),JSON.stringify({success:false,message:'已取消系统授权，本次未执行切换。'}));}
  else execFile(file,args,options,callback);
 }});
 function check(e){assert.equal(e.sender,win.webContents);assert.equal(e.senderFrame,win.webContents.mainFrame);assert.equal(new URL(e.senderFrame.url).origin,origin);}
 ipcMain.handle('timezone-status',e=>{check(e);return controller.status();});
 ipcMain.handle('timezone-apply',(e,target)=>{check(e);return controller.apply(target);});
 await require(path.join(desktop,'bundledUI.cjs')).install({session:win.webContents.session,root:path.join(resources,'ui'),getOrigin:()=>origin});
 await win.loadURL(origin+'/codex');
 for(let i=0;i<60;i++){
  if(await win.webContents.executeJavaScript("!!document.querySelector('.side a[href=\"/timezone\"]')"))break;
  await new Promise(r=>setTimeout(r,100));
 }
 await win.webContents.executeJavaScript("document.querySelector('.side a[href=\"/timezone\"]').click()");
 let state;
 for(let i=0;i<100;i++){
  state=await win.webContents.executeJavaScript("({route:location.pathname,text:document.querySelector('main').innerText,buttons:[...document.querySelectorAll('.timezone-city button')].filter(b=>!b.disabled).length})");
  if(state.buttons>0)break;await new Promise(r=>setTimeout(r,100));
 }
 assert.equal(state.route,'/timezone');assert.match(state.text,/当前电脑时区/);assert.ok(state.buttons>0,'Native state must load');
 await win.webContents.executeJavaScript("[...document.querySelectorAll('.timezone-city button')].find(b=>!b.disabled).click()");
 let error='';for(let i=0;i<50;i++){error=await win.webContents.executeJavaScript("document.querySelector('[role=alert]')?.textContent||''");if(error)break;await new Promise(r=>setTimeout(r,100));}
 assert.match(error,/已取消系统授权/);assert.equal(applies,1);
 await new Promise(r=>setTimeout(r,500));
 assert.match(await win.webContents.executeJavaScript("document.querySelector('[role=alert]')?.textContent||''"),/已取消系统授权/,'Status refresh must not erase an operation failure');
 win.setContentSize(390,844);await new Promise(r=>setTimeout(r,150));
 assert.ok(await win.webContents.executeJavaScript('document.documentElement.scrollWidth<=innerWidth'),'No horizontal overflow on narrow desktop');
 await win.loadURL(origin+'/timezone');
 for(let i=0;i<80;i++){if(await win.webContents.executeJavaScript("!!document.querySelector('.timezone-current strong') && document.querySelector('.timezone-current strong').textContent !== '--:--:--'"))break;await new Promise(r=>setTimeout(r,100));}
 assert.match(await win.webContents.executeJavaScript("document.querySelector('.timezone-current strong')?.textContent||''"),/^\d{2}:\d{2}:\d{2}$/);
 console.log(JSON.stringify({internalPage:true,nativeStateRead:true,cancellationFeedback:true,narrowLayout:true,directRouteReload:true,newToolWindow:false,systemTimezoneChanged:false}));
 win.destroy();app.exit(0);
}).catch(e=>{console.error(e.stack);app.exit(1);});
