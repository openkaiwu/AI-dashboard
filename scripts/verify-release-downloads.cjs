// Full public download verification; never treat headers or Range responses as
// byte-for-byte validation. Usage: node script BASE_URL LOCAL_MANIFEST REPORT
const fs=require('node:fs'),crypto=require('node:crypto');
(async()=>{
 const [baseUrl,manifestFile,reportFile]=process.argv.slice(2);
 const base=new URL(baseUrl);
 if(!['https:','http:'].includes(base.protocol))throw Error('HTTP(S) download URL required');
 const local=fs.readFileSync(manifestFile,'utf8');
 const remote=await fetch(new URL('SHA256SUMS-latest.txt',base),{signal:AbortSignal.timeout(30000),cache:'no-store'});
 if(!remote.ok||await remote.text()!==local)throw Error('Published manifest differs from local manifest');
 const files=local.trim().split(/\r?\n/).map(line=>{
  const match=line.match(/^([a-f0-9]{64})  ([^/\\]+)$/);
  if(!match||match[2].includes('..'))throw Error('Invalid manifest entry');
  return {name:match[2],sha256:match[1]};
 });
 const queue=[...files],results=[];
 async function worker(){
  while(queue.length){
   const file=queue.shift();let lastError;
   for(let attempt=0;attempt<2;attempt++){
    try{
     const start=Date.now(),response=await fetch(new URL(encodeURIComponent(file.name),base),{signal:AbortSignal.timeout(20*60*1000),cache:'no-store'});
     if(response.status!==200)throw Error('Expected complete HTTP 200, got '+response.status);
     const hash=crypto.createHash('sha256');let bytes=0;
     for await(const part of response.body){hash.update(part);bytes+=part.length;}
     const actual=hash.digest('hex');
     if(actual!==file.sha256)throw Error('SHA-256 mismatch');
     results.push({...file,bytes,seconds:Math.round((Date.now()-start)/1000),verified:true});
     fs.writeFileSync(reportFile,JSON.stringify({checkedAt:new Date().toISOString(),baseUrl:base.href,complete:results.length===files.length,files:results},null,2)+'\n');
     console.log(`Verified ${file.name}: ${bytes} bytes`);lastError=null;break;
    }catch(error){lastError=error;}
   }
   if(lastError)throw Error(file.name+': '+lastError.message);
  }
 }
 await Promise.all([worker(),worker(),worker()]);
 console.log(`FULL_DOWNLOAD_VERIFIED:${results.length}`);
})().catch(error=>{console.error(error.message);process.exitCode=1;});
