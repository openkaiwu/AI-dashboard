import { useEffect, useState, useRef } from "react";
import { enqueue, loadState, synchronize, rebuild, resolveConflict, visibleNotes, State, Note } from "../sync";
import {offlineStatus,OfflineStatus} from "../offline";
import { profile } from "../session";

export default function Sync() {
 const running=useRef(false);
 const [offline,setOffline]=useState<OfflineStatus>(offlineStatus());
 const [state,setState]=useState<State>({cursor:"",notes:{},pending:[],conflicts:[],lastSync:null});
 const [error,setError]=useState("");const [busy,setBusy]=useState(false);const [online,setOnline]=useState(navigator.onLine);
 const [title,setTitle]=useState("");const [body,setBody]=useState("");const [editing,setEditing]=useState<Note|null>(null);
 async function refresh(){try{setState(await loadState());}catch(e){setError(String(e));}}
 async function run(reset=false){if(running.current)return;running.current=true;setBusy(true);setError("");try{setState(await(reset?rebuild():synchronize()));}catch(e){setError(e instanceof TypeError?"无法连接服务器，本地修改已保留":e instanceof Error?e.message:"同步失败，待上传内容已保留");}finally{running.current=false;setBusy(false);}}
 useEffect(()=>{
 void refresh();void run();
 const cacheUpdate=()=>setOffline(offlineStatus());addEventListener("hub-offline",cacheUpdate);
 const update=()=>void refresh();const network=()=>{setOnline(navigator.onLine);if(navigator.onLine)void run();};
 addEventListener("hub-sync",update);addEventListener("online",network);addEventListener("offline",network);
 const timer=setInterval(()=>{if(navigator.onLine)void run();},30000);
 return()=>{removeEventListener("hub-offline",cacheUpdate);removeEventListener("hub-sync",update);removeEventListener("online",network);removeEventListener("offline",network);clearInterval(timer);};
 },[]);
 async function save(e:React.FormEvent){e.preventDefault();try{await enqueue(title,body,editing?.entity_id);setTitle("");setBody("");setEditing(null);if(online)void run();}catch(e){setError(String(e));}}
 const notes=visibleNotes(state);
 return <section>
 <div className="page-head"><div><p className="eyebrow">LOCAL FIRST / M0 FOUNDATION</p><h1>随时记录，跨端接续。</h1><p className="lead">便笺保存在此设备，联网后同步到你的服务器。</p></div><button className="btn" disabled={busy} onClick={()=>void run()}>{busy?"正在同步…":"立即同步 ↗"}</button></div>
 <div className="sync-banner"><span className={"status-dot "+(online?"":"offline")}/><strong>{online?"网络可用":"离线模式"}</strong><span>{profile().name}</span><span className="muted">{state.lastSync?"上次成功同步 "+new Date(state.lastSync).toLocaleString():"尚未同步"}</span></div>
 <div className="foundation-stats"><article><small>本地便笺</small><strong>{notes.length.toString().padStart(2,"0")}</strong><span>电脑 / 手机共享</span></article><article><small>待上传</small><strong>{state.pending.length.toString().padStart(2,"0")}</strong><span>重启后继续保留</span></article><article><small>需要处理</small><strong>{state.conflicts.length.toString().padStart(2,"0")}</strong><span>冲突内容不会被覆盖</span></article></div>
 {error&&<p className="error" role="alert">{error}</p>}
 {state.conflicts.map(c=><article className="conflict-card" key={c.operation.operation_id}><h3>这条便笺在另一台设备上有更新</h3><p>本地：{c.operation.op==="delete"?"删除便笺":c.operation.payload.title}</p><p className="muted">{c.operation.payload.body}</p><p>服务器：{c.current.op==="delete"?"已删除":c.current.payload.title}</p><p className="muted">{c.current.payload.body}</p><div className="actions"><button className="btn secondary" onClick={()=>void resolveConflict(c.operation.operation_id,false)}>采用服务器版本</button><button className="btn" onClick={()=>void resolveConflict(c.operation.operation_id,true).then(()=>run())}>保留我的修改并重试</button></div></article>)}
 <div className="sync-grid"><form className="panel note-compose" onSubmit={save}><p className="eyebrow">{editing?"EDIT NOTE":"QUICK CAPTURE"}</p><h2>{editing?"编辑便笺":"留给下一台设备"}</h2><label className="field"><span>标题</span><input placeholder="例如：下次继续整理 AI 服务" value={title} onChange={e=>setTitle(e.target.value)} maxLength={65} required/></label><label className="field"><span>内容</span><textarea placeholder="想法、待办、工作接续点…" rows={7} value={body} onChange={e=>setBody(e.target.value)} maxLength={4000}/></label><div className="actions"><button className="btn">保存到此设备</button>{editing&&<button className="btn ghost" type="button" onClick={()=>{setEditing(null);setTitle("");setBody("");}}>取消编辑</button>}</div><p className="muted compact">离线也能保存。待同步的便笺上传后可继续编辑。</p></form>
 <div className="notes-list">{notes.length===0?<div className="empty-notes"><div className="empty-icon">↗</div><h2>从一条便笺开始</h2><p>在电脑记下，打开手机接着看。<br/>两端登录同一服务器、同一账户即可。</p></div>:notes.map(n=>{
 const pending=state.pending.some(p=>p.entity_id===n.entity_id);const conflict=state.conflicts.some(c=>c.operation.entity_id===n.entity_id);
 return <article className="note-card" key={n.entity_id}><div className="note-meta"><span>{pending?"待同步":conflict?"有冲突":"已保存到服务器"}</span><span>v{n.version}</span></div><h3>{n.payload.title}</h3><p className="note-body">{n.payload.body||"没有正文"}</p><div className="actions"><button className="btn ghost" disabled={pending||conflict} onClick={()=>{setEditing(n);setTitle(n.payload.title);setBody(n.payload.body);}}>编辑</button><button className="btn ghost" disabled={pending||conflict} onClick={()=>{if(confirm("删除这条便笺？删除操作将同步到其他设备。"))void enqueue("","",n.entity_id,"delete").then(()=>{if(online)return run();}).catch(e=>setError(String(e)));}}>删除</button></div></article>;
 })}</div></div>
 <footer className="sync-footer"><span>{offline==="ready"?"离线启动缓存已就绪":offline==="preparing"?"正在准备离线启动缓存…":"此浏览器未就绪离线启动缓存；本地便笺仍会保留"}</span><button className="btn ghost" disabled={busy} onClick={()=>void run(true)}>重新拉取服务器记录</button></footer>
 </section>;
}

