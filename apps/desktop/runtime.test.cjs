const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const os=require('node:os');
const path=require('node:path');
const vm=require('node:vm');
const {EventEmitter}=require('node:events');

function runtimeFixture(t){
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'aihub-runtime-test-'));
 t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
 const dir=path.join(root,'runtime');fs.mkdirSync(dir);
 const exe=path.join(root,'codex.exe');fs.writeFileSync(exe,'');
 const binaries=path.join(root,'bin');fs.mkdirSync(binaries);fs.writeFileSync(path.join(binaries,'bridge.exe'),'');
 const cfg={server:'https://hub.example.com',device_id:'device',codex_path:exe,cursor_enabled:true,interval_seconds:300};
 fs.writeFileSync(path.join(dir,'bridge.json'),JSON.stringify(cfg));
 fs.writeFileSync(path.join(dir,'bridge-token.protected'),'fixture-token');
 fs.writeFileSync(path.join(dir,'sessions.protected'),JSON.stringify({default:{token:'test-token',device_id:'device'}}));
 const children=[],killed=[];let postgres=0;
 const electron={app:{getPath:()=>root,isPackaged:true},safeStorage:{decryptString:b=>b.toString(),encryptString:s=>Buffer.from(s),isEncryptionAvailable:()=>true}};
 const platform={binNames:()=>({bridgePackaged:'bridge.exe',serverPackaged:'server.exe'}),findCodex:async()=>exe,ensurePostgres:async()=>{postgres++;}};
 const fakeProcess={...process,resourcesPath:root,kill:(pid,signal)=>{if(signal===0&&!children.some(c=>c.pid===pid&&!c.killed))throw Error('ESRCH');if(signal!==0)killed.push(pid);}};
 const spawn=()=>{const child=new EventEmitter();child.pid=100+children.length;child.killed=false;child.kill=()=>{child.killed=true;killed.push(child.pid);child.emit('exit',0);};children.push(child);return child;};
 const module={exports:{}};
 vm.runInNewContext(fs.readFileSync(__dirname+'/runtime.cjs','utf8'),{require:n=>n==='electron'?electron:n==='node:child_process'?{spawn}:n==='./platform.cjs'?platform:n==='./credentialStore.cjs'?{encrypt:s=>Buffer.from(s),decrypt:b=>b.toString()}:require(n),module,__dirname,process:fakeProcess,Buffer,URL,fetch:async()=>({ok:true,json:async()=>({status:'ok'})}),AbortSignal,setTimeout,console});
 const wc={executeJavaScript:async()=>({id:'default',server:cfg.server})};
 return {runtime:module.exports,dir,wc,children,killed,postgres:()=>postgres};
}

test('overlapping bridge recovery launches one collector and restarts when config changes',async t=>{
 const h=runtimeFixture(t);
 await Promise.all([h.runtime.syncBridgeFromSession(h.wc,{}),h.runtime.syncBridgeFromSession(h.wc,{})]);
 assert.equal(h.children.length,1);
 await h.runtime.syncBridgeFromSession(h.wc,{});assert.equal(h.children.length,1);
 const file=path.join(h.dir,'bridge.json'),cfg=JSON.parse(fs.readFileSync(file));cfg.interval_seconds=600;fs.writeFileSync(file,JSON.stringify(cfg));
 await h.runtime.syncBridgeFromSession(h.wc,{});
 assert.equal(h.children.length,2);assert.ok(h.killed.includes(100));
 h.runtime.stopManaged();
});
test('blocked sessions stop collection rather than continuing to upload',async t=>{
 const h=runtimeFixture(t);await h.runtime.syncBridgeFromSession(h.wc,{});
 h.runtime.writeDesktopSession('default',{token:'test-token',device_id:'device',blocked:true});
 await h.runtime.syncBridgeFromSession(h.wc,{});assert.ok(h.killed.includes(100));assert.equal(h.children.length,1);
});
test('local startup calls the platform database bootstrap',async t=>{
 const h=runtimeFixture(t);h.runtime.setUserMode('local');await h.runtime.ensureReady();assert.equal(h.postgres(),1);
});
test('desktop installation identity survives browser data loss and accepts legacy identity once',t=>{
 const h=runtimeFixture(t),old='12345678-1234-1234-1234-123456789abc';
 assert.equal(h.runtime.installationID(old),old);
 assert.equal(h.runtime.installationID(null),old);
 assert.equal(h.runtime.installationID('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'),old);
});
