import { api, ApiError } from "./api";
import { scope } from "./session";

type Pending = {id:string;kind:"snapshot"|"account";targetId:string;body:Record<string,unknown>};
function db(): Promise<IDBDatabase> {
 return new Promise((resolve,reject)=>{
  const req=indexedDB.open("aihub-manual",1);
  req.onupgradeneeded=()=>req.result.createObjectStore("queue");
  req.onsuccess=()=>resolve(req.result);req.onerror=()=>reject(req.error);
 });
}
async function read(key:string):Promise<Pending[]> {
 const d=await db();return new Promise((resolve,reject)=>{
  const q=d.transaction("queue").objectStore("queue").get(key);
  q.onsuccess=()=>{d.close();resolve(q.result||[])};q.onerror=()=>{d.close();reject(q.error)};
 });
}
async function write(key:string,items:Pending[]) {
 const d=await db();return new Promise<void>((resolve,reject)=>{
  const tx=d.transaction("queue","readwrite");tx.objectStore("queue").put(items,key);
  tx.oncomplete=()=>{d.close();resolve()};tx.onerror=()=>{d.close();reject(tx.error)};
 });
}
export async function pendingManualCount(){return (await read(scope())).length;}
export async function clearManualQueue(){await write(scope(),[]);}
export async function enqueueManual(bucketId:string,body:Record<string,unknown>){
 const key=scope();const id=typeof body.operation_id==='string'?body.operation_id:crypto.randomUUID();const entry:Pending={id,kind:"snapshot",targetId:bucketId,body:{...body,operation_id:id}};
 await navigator.locks.request("manual:"+key,async()=>{const items=await read(key);if(!items.some(x=>x.id===entry.id))await write(key,[...items,entry]);});
 await flushManual();
 return entry;
}
export async function enqueueAccountPatch(accountId:string,body:Record<string,unknown>){
 const key=scope(),entry:Pending={id:crypto.randomUUID(),kind:"account",targetId:accountId,body};
 await navigator.locks.request("manual:"+key,async()=>write(key,[...await read(key),entry]));
 await flushManual();return entry;
}
export async function flushManual(){
 const key=scope();return navigator.locks.request("manual:"+key,async()=>{
  const items=await read(key);
  for(const item of items){
   if(scope()!==key)throw new Error("账户已切换");
   try{if(item.kind==="account")await api.patchAccount(item.targetId,item.body);else await api.manualSnapshot(item.targetId,item.body)}catch(e){
    if(e instanceof ApiError && e.status>=400 && e.status<500 && e.status!==408 && e.status!==429)throw e;
    return false;
   }
   await write(key,(await read(key)).filter(x=>x.id!==item.id));
  }
  return true;
 });
}
