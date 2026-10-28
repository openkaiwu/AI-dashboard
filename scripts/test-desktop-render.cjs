// Run with the Electron executable, not Node. Uses an isolated, disposable profile.
const {app, BrowserWindow, ipcMain} = require('electron');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const assert = require('node:assert/strict');
const profile = fs.mkdtempSync(path.join(os.tmpdir(), 'aihub-render-'));
app.setPath('userData', profile);
const desktop = path.resolve(process.env.AIHUB_TEST_DESKTOP_ROOT || path.join(__dirname, '../apps/desktop'));
const root = path.resolve(process.env.AIHUB_TEST_UI_ROOT || path.join(desktop, 'resources/ui'));
const errors = [];
let origin = 'https://aihub.example.invalid';
let win;
const deadline = setTimeout(() => { console.error('FAIL: renderer timed out'); app.exit(1); }, 40000);
app.whenReady().then(async () => {
  win = new BrowserWindow({show:false, webPreferences:{preload:path.join(desktop,'preload.cjs'),sandbox:true,contextIsolation:true,nodeIntegration:false}});
  win.webContents.on('console-message', event => {
    if (event.level === 'error' || /Content Security Policy/i.test(event.message)) errors.push(event.message);
  });
  win.webContents.on('preload-error', (_e, _file, error) => errors.push(error.message));
  ipcMain.on('desktop-info', e => { e.returnValue = {version:'render-test',bundledUI:true,platform:process.platform}; });
  ipcMain.on('session-get', e => { e.returnValue = null; });
  ipcMain.on('installation-id', e => { e.returnValue = 'render-test-installation'; });
  await require(path.join(desktop,'bundledUI.cjs')).install({session:win.webContents.session,root,getOrigin:()=> origin});
  await win.loadURL(origin+'/codex');
  let state;
  for (let attempt=0; attempt<30; attempt++) {
    state = await win.webContents.executeJavaScript(`({text:document.getElementById('root')?.innerText || '',inputs:document.querySelectorAll('input').length,desktop:window.aihubDesktop?.info?.().bundledUI,route:location.pathname})`);
    if (state.inputs > 0 || errors.length) break;
    await new Promise(resolve=>setTimeout(resolve,100));
  }
  assert.equal(errors.length,0,errors.join('\n'));
  assert.ok(state.inputs > 0, 'Login form must render, not just a successful HTML navigation: '+JSON.stringify(state));
  assert.equal(state.desktop,true);
  assert.equal(state.route,'/login');
  // Both supported origins must render, with no server reachable or signed-in account.
  origin = 'http://aihub.example.invalid';
  await win.loadURL(origin+'/codex');
  for (let attempt=0; attempt<30; attempt++) {
    if (await win.webContents.executeJavaScript("document.querySelectorAll('input').length > 0")) break;
    await new Promise(resolve=>setTimeout(resolve,100));
  }
  assert.ok(await win.webContents.executeJavaScript("document.querySelectorAll('input').length > 0"));
  assert.equal(errors.length,0,errors.join('\n'));
  // An injected inline script must still be blocked, even though the shipped bundle runs.
  await win.webContents.executeJavaScript(`{const s=document.createElement('script');s.textContent='window.__unexpectedInline = true';document.head.appendChild(s);}`);
  assert.equal(await win.webContents.executeJavaScript('window.__unexpectedInline'), undefined);
  // If startup fails in a future bundle, the external guard gives a working retry.
  await win.webContents.executeJavaScript("document.getElementById('root').replaceChildren()");
  for (let attempt=0; attempt<120; attempt++) {
    if (await win.webContents.executeJavaScript("!!document.querySelector('#root [role=alert] button')")) break;
    await new Promise(resolve=>setTimeout(resolve,100));
  }
  assert.match(await win.webContents.executeJavaScript("document.getElementById('root').innerText"),/界面未能启动/);
  const navigation = new Promise(resolve=>win.webContents.once('did-finish-load',resolve));
  await win.webContents.executeJavaScript("document.querySelector('#root [role=alert] button').click()");
  await navigation;
  for (let attempt=0; attempt<30; attempt++) {
    if (await win.webContents.executeJavaScript("document.querySelectorAll('input').length > 0")) break;
    await new Promise(resolve=>setTimeout(resolve,100));
  }
  assert.ok(await win.webContents.executeJavaScript("document.querySelectorAll('input').length > 0"));
  console.log(JSON.stringify({result:'PASS',route:state.route,loginInputs:state.inputs,desktopBridge:state.desktop,httpAndHttps:true,untrustedInlineBlocked:true,failureRecovery:true}));
  clearTimeout(deadline);
  win.destroy();
  app.exit(0);
}).catch(error=>{console.error(error.stack); clearTimeout(deadline); app.exit(1);});
