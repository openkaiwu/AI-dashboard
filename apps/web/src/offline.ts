export type OfflineStatus="preparing"|"ready"|"unavailable";
let status:OfflineStatus="preparing";
export function offlineStatus(){return status;}
function publish(value:OfflineStatus){status=value;dispatchEvent(new Event("hub-offline"));}
export async function prepareOffline(){
 if(!import.meta.env.PROD || !("serviceWorker" in navigator)){publish("unavailable");return;}
 try{
  if(await caches.match(location.origin+"/"))publish("ready");
  const registration=await navigator.serviceWorker.register("/sw.js",{updateViaCache:"none"});
  await registration.update();
  await Promise.race([navigator.serviceWorker.ready,new Promise((_,reject)=>setTimeout(()=>reject(new Error("timeout")),15000))]);
  const response=await caches.match(location.origin+"/");
  publish(response?"ready":"unavailable");
 }catch{if(status!=="ready")publish("unavailable");}
}
