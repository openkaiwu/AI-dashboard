// Pure helpers for desktop collector control and authorized scan directories.
// File formats are owned here so runtime.cjs stays Electron-bound and testable
// logic stays runnable without an Electron fixture.
const fs=require('node:fs');
const path=require('node:path');

const MAX_SCAN_ROOTS=8;
const MAX_ROOT_LENGTH=1024;
const PAUSE_FILE='bridge-paused.json';
const SCAN_ROOTS_FILE='scan-directories.json';

function readJson(file,fallback=null){try{return JSON.parse(fs.readFileSync(file,'utf8'));}catch{return fallback;}}
function writePrivate(file,value){fs.mkdirSync(path.dirname(file),{recursive:true});const tmp=file+'.'+Date.now()+'.tmp';fs.writeFileSync(tmp,JSON.stringify(value),{mode:0o600});fs.renameSync(tmp,file);}

function pauseFile(dir){return path.join(dir,PAUSE_FILE);}
function isPaused(dir){return readJson(pauseFile(dir))?.paused===true;}
function setPaused(dir,paused){
 if(!paused){try{fs.unlinkSync(pauseFile(dir));}catch{}return;}
 writePrivate(pauseFile(dir),{paused:true,at:new Date().toISOString()});
}

function normalizeRoots(inputs){
 if(!Array.isArray(inputs))throw Error('授权目录必须是列表');
 const roots=[];
 for(const raw of inputs){
  if(typeof raw!=='string'||raw.length===0||raw.length>MAX_ROOT_LENGTH||raw.includes('\0'))throw Error('无效的授权目录');
  if(!path.isAbsolute(raw))throw Error('授权目录必须是绝对路径：'+raw.slice(0,80));
  const cleaned=path.normalize(raw);
  if(!roots.includes(cleaned))roots.push(cleaned);
 }
 if(roots.length>MAX_SCAN_ROOTS)throw Error('授权目录最多 '+MAX_SCAN_ROOTS+' 个');
 return roots;
}
function scanRootsFile(dir){return path.join(dir,SCAN_ROOTS_FILE);}
function readScanRoots(dir){try{return normalizeRoots(readJson(scanRootsFile(dir))?.roots||[]);}catch{return [];}}
function writeScanRoots(dir,roots){writePrivate(scanRootsFile(dir),{roots,updated_at:new Date().toISOString()});}
function addScanRoots(dir,inputs){
 const existing=readScanRoots(dir),added=normalizeRoots(inputs),merged=[...existing];
 for(const root of added)if(!merged.includes(root))merged.push(root);
 if(merged.length>MAX_SCAN_ROOTS)throw Error('授权目录最多 '+MAX_SCAN_ROOTS+' 个');
 writeScanRoots(dir,merged);return {roots:merged};
}
function removeScanRoot(dir,root){
 const target=path.normalize(String(root||''));
 const roots=readScanRoots(dir).filter(entry=>entry!==target);
 if(roots.length===readScanRoots(dir).length)return {roots,removed:false};
 writeScanRoots(dir,roots);return {roots,removed:true};
}

// Reads at most the tail of a collector log so diagnostics stay bounded.
function lastLogLine(file,maxBytes=4096){
 try{
  const stat=fs.statSync(file);if(!stat.isFile()||stat.size===0)return '';
  const handle=fs.openSync(file,'r');
  try{
   const start=Math.max(0,stat.size-maxBytes);
   const buffer=Buffer.alloc(stat.size-start);
   fs.readSync(handle,buffer,0,buffer.length,start);
   const lines=buffer.toString('utf8').split(/\r?\n/).filter(line=>line.trim());
   return (lines[lines.length-1]||'').slice(0,400);
  }finally{fs.closeSync(handle);}
 }catch{return '';}
}

module.exports={MAX_SCAN_ROOTS,isPaused,setPaused,normalizeRoots,readScanRoots,addScanRoots,removeScanRoot,lastLogLine};
