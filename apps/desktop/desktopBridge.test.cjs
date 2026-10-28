const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const os=require('node:os');
const path=require('node:path');
const {isPaused,setPaused,normalizeRoots,readScanRoots,addScanRoots,removeScanRoot,lastLogLine}=require('./desktopBridge.cjs');

function tempDir(t){
 const root=fs.mkdtempSync(path.join(os.tmpdir(),'aihub-desktop-bridge-'));
 t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
 return root;
}
test('pause state persists until cleared',t=>{
 const dir=tempDir(t);
 assert.equal(isPaused(dir),false);
 setPaused(dir,true);assert.equal(isPaused(dir),true);
 setPaused(dir,false);assert.equal(isPaused(dir),false);
 setPaused(dir,false);assert.equal(isPaused(dir),false);
});
test('scan roots require absolute paths, dedupe and cap at eight',t=>{
 assert.throws(()=>normalizeRoots('relative'),/列表/);
 assert.throws(()=>normalizeRoots(['ok']),/绝对路径/);
 assert.throws(()=>normalizeRoots([123]),/无效/);
 const root=tempDir(t);
 const first=path.join(root,'a'),second=path.join(root,'b'),nested=path.join(first,'..','a');
 const {roots}=addScanRoots(root,[first,second,nested]);
 assert.deepEqual(roots,[first,second]);
 assert.throws(()=>addScanRoots(root,Array.from({length:9},(_,i)=>path.join(root,'d'+i))),/最多 8 个/);
 assert.throws(()=>normalizeRoots([first+String.fromCharCode(0)]),/无效/);
});
test('removeScanRoot removes only the exact authorized directory',t=>{
 const dir=tempDir(t);
 const keep=path.join(dir,'keep'),drop=path.join(dir,'drop');
 addScanRoots(dir,[keep,drop]);
 const {roots,removed}=removeScanRoot(dir,drop);
 assert.equal(removed,true);assert.deepEqual(roots,[keep]);
 assert.deepEqual(readScanRoots(dir),[keep]);
 assert.equal(removeScanRoot(dir,drop).removed,false);
});
test('lastLogLine returns the final bounded log entry',t=>{
 const dir=tempDir(t);
 const file=path.join(dir,'bridge.log');
 assert.equal(lastLogLine(file),'');
 fs.writeFileSync(file,'line one\n\nline two\n');
 assert.equal(lastLogLine(file),'line two');
 fs.writeFileSync(file,'x'.repeat(8192)+'\nfinal entry\n');
 assert.equal(lastLogLine(file),'final entry');
});
