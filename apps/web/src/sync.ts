import { request, ApiError } from "./api";
import { scope } from "./session";

export type Note = { entity_id: string; version: number; op: "put" | "delete"; payload: {title:string;body:string}; changed_at?:string; seq?:number };
export type Operation = { protocol:1;operation_id:string;entity:"note";entity_id:string;base_version:number;op:"put"|"delete";payload:Note["payload"] };
export type Conflict = { operation:Operation; current:Note };
export type State = { cursor:string;notes:Record<string,Note>;pending:Operation[];conflicts:Conflict[];lastSync:string|null };
const empty = ():State => ({cursor:"",notes:{},pending:[],conflicts:[],lastSync:null});
let dbPromise:Promise<IDBDatabase> | null = null;
function database() {
 return dbPromise ||= new Promise<IDBDatabase>((resolve,reject) => {
 const q=indexedDB.open("aihub-m0",1);
 q.onupgradeneeded=()=>q.result.createObjectStore("states");q.onsuccess=()=>resolve(q.result);q.onerror=()=>reject(q.error);
 });
}
async function read(key:string):Promise<State> {
 const db=await database();return new Promise((resolve,reject)=>{
 const q=db.transaction("states").objectStore("states").get(key);q.onsuccess=()=>resolve(q.result||empty());q.onerror=()=>reject(q.error);
 });
}
async function write(key:string,state:State) {
 const db=await database();return new Promise<void>((resolve,reject)=>{
 const tx=db.transaction("states","readwrite");tx.objectStore("states").put(state,key);tx.oncomplete=()=>resolve();tx.onerror=()=>reject(tx.error);tx.onabort=()=>reject(tx.error);
 });
}
export async function loadState(){return read(scope());}
async function change(key:string,fn:(s:State)=>void) {
 await navigator.locks.request("hub-state:"+key,async()=>{const s=await read(key);fn(s);await write(key,s);});
 dispatchEvent(new Event("hub-sync"));
}
export async function enqueue(title:string,body:string,id:string=crypto.randomUUID(),op:"put"|"delete"="put") {
 const key=scope();
 await change(key,s=>{
 if(s.pending.some(p=>p.entity_id===id)||s.conflicts.some(c=>c.operation.entity_id===id))throw new Error("这条便笺仍有待同步或冲突内容，请先处理");
 s.pending.push({protocol:1,operation_id:crypto.randomUUID(),entity:"note",entity_id:id,base_version:s.notes[id]?.version||0,op,payload:{title,body}});
 });
}
export async function resolveConflict(id:string,keepLocal:boolean) {
 const key=scope();
 await change(key,s=>{
 const c=s.conflicts.find(x=>x.operation.operation_id===id);if(!c)return;
 if(keepLocal)s.pending.push({...c.operation,operation_id:crypto.randomUUID(),base_version:s.notes[c.operation.entity_id]?.version||c.current.version});
 s.conflicts=s.conflicts.filter(x=>x.operation.operation_id!==id);
 });
}
export async function synchronize() {
 const key=scope();
 return navigator.locks.request("hub-sync:"+key,async()=>{
 const checkScope=()=>{if(scope()!==key)throw new Error("账户已切换，请重新同步");};
 checkScope();let state=await read(key);
 for(const operation of state.pending){
 checkScope();
 const result=await request<{status:string;event?:Note;current?:Note}>("/api/v1/sync/push",{method:"POST",body:JSON.stringify(operation)});
 await change(key,s=>{
 s.pending=s.pending.filter(p=>p.operation_id!==operation.operation_id);
 if(result.status==="conflict"&&result.current){s.conflicts.push({operation,current:result.current});if(result.current.version)s.notes[operation.entity_id]=result.current;}
 if(result.event&&result.event.version>=(s.notes[operation.entity_id]?.version||0))s.notes[operation.entity_id]=result.event;
 });
 }
 let reset=false;
 for(;;){
 checkScope();state=await read(key);
 try{
 const page=await request<{cursor:string;events:Note[];has_more:boolean}>("/api/v1/sync/pull?cursor="+encodeURIComponent(state.cursor));
 await change(key,s=>{for(const e of page.events){if(e.version>=(s.notes[e.entity_id]?.version||0))s.notes[e.entity_id]=e;}s.cursor=page.cursor;if(!page.has_more)s.lastSync=new Date().toISOString();});
 if(!page.has_more)break;
 }catch(e){
 if(e instanceof ApiError&&e.code==="cursor_reset"&&!reset){reset=true;await change(key,s=>{s.cursor="";s.notes={};});continue;}throw e;
 }
 }
 return read(key);
 });
}
export async function rebuild(){await change(scope(),s=>{s.cursor="";s.notes={};});return synchronize();}
export function visibleNotes(s:State) {
 const map={...s.notes};
 for(const p of s.pending)map[p.entity_id]={entity_id:p.entity_id,version:p.base_version,op:p.op,payload:p.payload};
 return Object.values(map).filter(n=>n.op!=="delete");
}
