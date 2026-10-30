const {test}=require('node:test'),assert=require('node:assert/strict');
const {createController}=require('./timeZoneTool.cjs');
const base={root:'resources',platform:'win32',exists:()=>true};
test('headless helper uses fixed executable and validated target, with no shell',async()=>{
 const calls=[];const controller=createController({...base,execute:(file,args,options,cb)=>{calls.push({file,args,options});cb(null,JSON.stringify({id:'China Standard Time'}));}});
 await controller.status();await controller.apply('la');
 assert.deepEqual(calls.map(c=>c.args),[['--status'],['--switch','la']]);
 assert.ok(calls.every(c=>c.options.windowsHide&&!c.options.shell&&c.file.endsWith('TimeZoneSwitch.exe')));
 assert.equal(calls[1].options.timeout,undefined,'Do not abandon a privileged helper mid-operation');
 await assert.rejects(controller.apply('bad;command'),/只支持/);assert.equal(calls.length,2);
});
test('concurrent switches are rejected and cancellation is reported without success',async()=>{
 let callback;const controller=createController({...base,execute:(_f,_a,_o,cb)=>{callback=cb;}});
 const first=controller.apply('cn');await assert.rejects(controller.apply('la'),/正在切换/);
 callback(Error('exit 1'),JSON.stringify({success:false,message:'已取消系统授权'}));await assert.rejects(first,/已取消系统授权/);
 const next=controller.apply('cn');callback(null,JSON.stringify({success:true}));await next;
});
test('unsupported platform, missing helper and invalid output fail clearly',async()=>{
 await assert.rejects(createController({...base,platform:'linux'}).status(),/Windows/);
 await assert.rejects(createController({...base,exists:()=>false}).status(),/未找到/);
 await assert.rejects(createController({...base,execute:(_f,_a,_o,cb)=>cb(null,'bad')}).status(),/无法读取/);
});
