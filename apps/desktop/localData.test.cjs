const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs'),os=require('node:os'),path=require('node:path');
const data=require('./localData.cjs');
function fixture(t){const dir=fs.mkdtempSync(path.join(os.tmpdir(),'aihub-local-'));t.after(()=>fs.rmSync(dir,{recursive:true,force:true}));data.seed(dir,{preferences:{...data.preferences(dir).preferences,quiet_start:0,quiet_end:0},plan:{plan_type:'plus'}},{});return dir;}

test('manual sync includes only bounded recent quota evidence, not other local fields',t=>{
 const dir=fixture(t),now=Date.now();
 const sample={observed_at:new Date(now).toISOString(),source:'codex_app_server',status:'ok',buckets:[],private_text:'must-not-upload'};
 data.write(path.join(dir,'codex-report.json'),{snapshot:sample,history:[{...sample,observed_at:new Date(now-20*60000).toISOString()},sample]});
 const backup=data.backup(dir);
 assert.equal(backup.recent_snapshots.length,1);
 assert.equal(backup.recent_snapshots[0].private_text,undefined);
});
test('local aggregates persist across module reload and are isolated by user/server',t=>{
 const dir=fixture(t),hour=new Date().toISOString().slice(0,13)+':00:00Z';
 data.write(path.join(dir,'rollups.json'),{last:new Date().toISOString(),rows:{x:{hour,duration:10080,consumed_pp:12,samples:3,resets:1}}});
 assert.equal(data.settings(dir).auto_sync,false);
 assert.equal(data.consumption(dir,'window=10080&granularity=hourly').hourly[0].consumed_pp,12);
 assert.notEqual(data.directory(dir,'https://one','a'),data.directory(dir,'https://one','b'));
 assert.notEqual(data.directory(dir,'https://one','a'),data.directory(dir,'https://two','a'));
 assert.equal(require('./localData.cjs').backup(dir).consumption['10080:daily'].series[0].consumed_pp,12);
});

test('local consumption retains actual observation time and never substitutes generation time',t=>{
 const dir=fixture(t),observed='2026-10-20T01:00:00Z';
 assert.equal(data.consumption(dir,'').data_as_of,undefined);
 data.write(path.join(dir,'rollups.json'),{last:observed,rows:{}});
 assert.equal(data.consumption(dir,'').data_as_of,observed);
});
test('a delivered local risk survives reload and transient recovery; list actions never rearm presentation',t=>{
 const dir=fixture(t),advice={key:'cycle:low',kind:'slow',title:'low',body:'risk',priority:3,interrupt:true};
 const report={snapshot:{observed_at:new Date().toISOString(),status:'ok'},history:[],analysis:{state:'slow',metrics:[],advice:[advice]}};
 data.write(path.join(dir,'codex-report.json'),report);
 const a=data.overview(dir).alerts[0],url='/api/v1/codex/alerts/'+encodeURIComponent(a.id);
 const claim=data.request(dir,url+'/delivery','POST',{action:'claim',token:a.notified_at});
 data.request(dir,url+'/delivery','POST',{action:'delivered',claim_id:claim.claim_id});
 assert.equal(data.request(dir,url+'/delivery','POST',{action:'claim',token:a.notified_at}).reason,'delivered');
 data.write(path.join(dir,'codex-report.json'),{...report,analysis:{...report.analysis,advice:[]}});data.overview(dir);
 data.write(path.join(dir,'codex-report.json'),report);
 assert.equal(data.overview(dir).alerts[0].notified_at,a.notified_at);
 data.request(dir,url,'POST',{action:'snooze'});
 assert.equal(data.overview(dir).alerts[0].notified_at,a.notified_at);
 data.request(dir,url,'POST',{action:'restore'});
 assert.equal(data.request(dir,url+'/delivery','POST',{action:'claim',token:a.notified_at}).reason,'delivered');
});
test('Cursor local risks honor disabled rules and use the same cycle key as server',t=>{
 const dir=fixture(t),reset='2026-11-01T00:00:00Z';
 data.write(path.join(dir,'cloud-dashboard.json'),{accounts:[{id:'a',provider:{slug:'cursor'},buckets:[{id:'b',scope_key:'plan'}]}]});
 data.write(path.join(dir,'cursor-latest.json'),{status:'ok',observed_at:new Date().toISOString(),reset_at:reset,buckets:[{scope_key:'plan',used_percent:95}]});
 const rules={rules:[{id:'r',enabled:false,rule_type:'quota.low',params:{ratio:0.1}}]};data.write(path.join(dir,'notification-rules.json'),rules);
 assert.equal(data.request(dir,'/api/v1/notifications').notifications.length,0);
 rules.rules[0].enabled=true;data.write(path.join(dir,'notification-rules.json'),rules);
 assert.equal(data.request(dir,'/api/v1/notifications').notifications[0].dedupe_key,'quota.low:b:'+reset);
});
test('migration merges consumption without counting the shared hour twice',t=>{
 const dir=fixture(t),hour=new Date().toISOString().slice(0,13)+':00:00Z',day=new Date(Date.parse(hour)+480*60000).toISOString().slice(0,10);
 data.write(path.join(dir,'baseline.json'),{'10080:hourly':{hourly:[{hour,samples:2,consumed_pp:3}]},'10080:daily':{series:[{bucket_start:day,consumed_pp:3,coverage_hours:1}]}});
 data.write(path.join(dir,'rollups.json'),{last:new Date().toISOString(),rows:{x:{hour,duration:10080,consumed_pp:2,samples:2,resets:0}}});
 const row=data.consumption(dir,'window=10080&granularity=daily').series[0];assert.equal(row.consumed_pp,5);assert.equal(row.coverage_hours,1);
});
test('upload queue survives reload, retries keep one operation id, acceptance clears pending',t=>{
 const dir=fixture(t);
 const entry=data.queueEnqueue(dir);
 assert.match(entry.id,/-1$/);
 // the queue is durable: a fresh module load still sees the same pending reception
 const reloaded=require('./localData.cjs').queueStatus(dir);
 assert.equal(reloaded.operation_id,entry.id);
 assert.equal(reloaded.pending,true);
 // a network failure keeps the id (server-side dedupe of the lost response)
 data.queueFail(dir,entry.id,'网络中断',{backoff_ms:30000});
 const failed=data.queueStatus(dir);
 assert.equal(failed.operation_id,entry.id);
 assert.equal(failed.attempts,1);
 assert.equal(failed.needs_auth,false);
 assert.ok(failed.next_attempt&&Date.parse(failed.next_attempt)>Date.now());
 // an auth failure stops automatic retries
 data.queueFail(dir,entry.id,'登录已过期',{backoff_ms:0,needs_auth:true});
 assert.equal(data.queueStatus(dir).needs_auth,true);
 // acceptance clears the pending entry; the next enqueue gets a new id
 data.queueAccept(dir,entry.id);
 assert.equal(data.queueStatus(dir).pending,false);
 const next=data.queueEnqueue(dir);
 assert.equal(next.id,entry.id.replace(/-1$/,'-2'));
 assert.equal(data.backup(dir,next.id).operation_id,next.id);
 // accepting a stale id must not clear a newer pending reception
 data.queueAccept(dir,entry.id);
 assert.equal(data.queueStatus(dir).pending,true);
 // legacy uploads without an operation id stay valid
 assert.equal(data.backup(dir).operation_id,undefined);
});
