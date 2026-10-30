const fs=require('node:fs'),path=require('node:path');
const {execFile}=require('node:child_process');
function createController({root,platform=process.platform,execute=execFile,exists=fs.existsSync}){
 let switching=null;
 function invoke(args){
  if(platform!=='win32')return Promise.reject(Error('时区切换目前支持 Windows 10 / 11。'));
  const file=path.join(root,'timezone','TimeZoneSwitch.exe');
  if(!exists(file))return Promise.reject(Error('未找到内置时区组件，请更新 AI Hub。'));
  return new Promise((resolve,reject)=>execute(file,args,{windowsHide:true,maxBuffer:65536,...(args[0]==='--status'?{timeout:15000}:{})},(error,stdout)=>{
   let data;try{data=JSON.parse(String(stdout).replace(/^\uFEFF/,''));}catch{return reject(Error('无法读取系统时区，请稍后重试。'));}
   if(data.success===false)return reject(Error(data.message||'系统未允许切换时区。'));
   if(error)return reject(Error('时区组件执行失败。'));
   resolve(data);
  }));
 }
 return {status:()=>invoke(['--status']),apply:target=>{
  if(!['cn','la'].includes(target))return Promise.reject(Error('只支持中国或洛杉矶时区。'));
  if(switching)return Promise.reject(Error('正在切换时区，请等待完成。'));
  switching=invoke(['--switch',target]).finally(()=>{switching=null;});return switching;
 }};
}
module.exports={createController};
