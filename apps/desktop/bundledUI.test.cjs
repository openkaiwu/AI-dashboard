const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs/promises');
const path=require('node:path');
const os=require('node:os');
const {createHash}=require('node:crypto');
const {createHandler}=require('./bundledUI.cjs');
test('bundled UI loads offline at the existing origin while API retains network semantics',async t=>{
 const root=await fs.mkdtemp(path.join(os.tmpdir(),'hub-ui-'));
 t.after(()=>fs.rm(root,{recursive:true,force:true}));
 await fs.writeFile(path.join(root,'index.html'),'<h1>Bundled app</h1>');
 let forwarded=[];
 const handle=createHandler({root,getOrigin:()=> 'https://hub.example.com',forward:async r=>{forwarded.push(r);return new Response('upstream',{status:401});}});
 const page=await handle(new Request('https://hub.example.com/conversations'));
 assert.equal(page.status,200);assert.match(await page.text(),/Bundled/);assert.equal(forwarded.length,0);
 const req=new Request('https://hub.example.com/api/v1/conversations',{headers:{Authorization:'Bearer fixture'}});
 assert.equal((await handle(req)).status,401);assert.equal(forwarded[0],req);
 assert.equal((await handle(new Request('https://hub.example.com/sw.js'))).status,404);
 for(const p of ['/runtime/sessions.protected','/assets/%2e%2e%2fcredentials','/assets/a%5cb.js','/resources/app-config.json']) assert.ok((await handle(new Request('https://hub.example.com'+p))).status>=400,p);
 assert.equal((await handle(new Request('https://hub.example.com/codex',{method:'POST'}))).status,405);
 assert.equal((await handle(new Request('https://hub.example.com/codex',{method:'HEAD'}))).body,null);
});

test('CSP authorizes the exact bundled inline module without allowing arbitrary inline scripts',async t=>{
 const root=await fs.mkdtemp(path.join(os.tmpdir(),'hub-ui-csp-'));
 t.after(()=>fs.rm(root,{recursive:true,force:true}));
 const script='\r\nwindow.started = true;\r\n';
 await fs.writeFile(path.join(root,'index.html'),'<html><head><script type="module">'+script+'</script></head><body><div id="root"></div></body></html>');
 const handle=createHandler({root,getOrigin:()=> 'https://hub.example.com',forward:()=>{throw Error('Unexpected network request');}});
 const page=await handle(new Request('https://hub.example.com/codex'));
 const csp=page.headers.get('Content-Security-Policy');
 const hash=createHash('sha256').update(script.replace(/\r\n/g,'\n')).digest('base64');
 assert.ok(csp.includes("script-src 'self' 'sha256-"+hash+"'"));
 assert.doesNotMatch(csp.split('script-src ')[1].split(';')[0],/unsafe-inline|unsafe-eval/);
 assert.match(await page.text(),/src="\/__desktop_startup.js"/);
 assert.equal((await handle(new Request('https://hub.example.com/codex',{method:'HEAD'}))).headers.get('Content-Security-Policy'),csp);
 const guard=await handle(new Request('https://hub.example.com/__desktop_startup.js'));
 assert.equal(guard.status,200);assert.match(await guard.text(),/界面未能启动/);
 // A changed bundle gets a new hash on the next navigation.
 await fs.writeFile(path.join(root,'index.html'),'<script>window.updated = true;</script>');
 assert.notEqual((await handle(new Request('https://hub.example.com/codex'))).headers.get('Content-Security-Policy'),csp);
});
