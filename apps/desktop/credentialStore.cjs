const {execFileSync}=require('node:child_process');
const prefix=Buffer.from('AIHub.DPAPI.1\n');
// Bind Windows credentials to the OS user, independently of Chromium profile keys.
// Values travel over stdin/stdout pipes, never command arguments or application logs.
function dpapi(operation,input){
 const code=`$ErrorActionPreference='Stop';Add-Type -AssemblyName System.Security;$inputBytes=[Convert]::FromBase64String([Console]::In.ReadToEnd());$entropy=[Text.Encoding]::UTF8.GetBytes('AIHub.credentials.v1');$out=[System.Security.Cryptography.ProtectedData]::${operation}($inputBytes,$entropy,[System.Security.Cryptography.DataProtectionScope]::CurrentUser);[Console]::Write([Convert]::ToBase64String($out))`;
 try{return Buffer.from(execFileSync('powershell.exe',['-NoProfile','-NonInteractive','-Command',code],{input:input.toString('base64'),windowsHide:true,timeout:15000,stdio:['pipe','pipe','pipe']}).toString().trim(),'base64');}
 catch{throw Error('系统凭据保护不可用');}
}
function encrypt(value,safeStorage){
 if(process.platform==='win32')return Buffer.concat([prefix,dpapi('Protect',Buffer.from(value,'utf8'))]);
 if(!safeStorage.isEncryptionAvailable())throw Error('系统凭据保护不可用');
 return safeStorage.encryptString(value);
}
function decrypt(value,safeStorage){
 if(value.subarray(0,prefix.length).equals(prefix)){
  if(process.platform!=='win32')throw Error('此凭据属于 Windows 账户');
  return dpapi('Unprotect',value.subarray(prefix.length)).toString('utf8');
 }
 return safeStorage.decryptString(value);
}
module.exports={encrypt,decrypt};
