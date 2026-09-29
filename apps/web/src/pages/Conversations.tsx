import {useEffect,useState} from "react";
import {request,requestRaw} from "../api";

type Conversation={id:string;title:string;provider_slug:string;message_count:number;branch_count:number;project_name:string|null;updated_at:string};
type Project={id:string;name:string;conversations:number};
type ImportRow={id:string;source_type:string;status:string;conversations_created:number;conversations_deduplicated:number;messages_imported:number;error:string};
type Detail={id:string;title:string;provider_slug:string;project_id:string|null;branches:{id:string;name:string;messages:{id:string;parent_id:string|null;role:string;content:string}[]}[]};

export default function Conversations(){
 const[convs,setConvs]=useState<Conversation[]>([]),[projects,setProjects]=useState<Project[]>([]);
 const[imports,setImports]=useState<ImportRow[]>([]),[detail,setDetail]=useState<Detail|null>(null);
 const[source,setSource]=useState("chatgpt_export"),[error,setError]=useState(""),[note,setNote]=useState("");
 const[projectName,setProjectName]=useState("");

 const reload=()=>{void request<{conversations:Conversation[]}>("/api/v1/conversations").then(x=>setConvs(x.conversations)).catch(e=>setError(String(e)));
  void request<{projects:Project[]}>("/api/v1/projects").then(x=>setProjects(x.projects)).catch(()=>{});
  void request<{imports:ImportRow[]}>("/api/v1/imports").then(x=>setImports(x.imports)).catch(()=>{});};
 useEffect(reload,[]);

 async function open(id:string){
  setError("");try{setDetail(await request<Detail>("/api/v1/conversations/"+id));}catch(e){setError(String(e));}
 }
 async function importFile(file:File){
  setError("");setNote("");
  try{
   const res=await requestRaw("/api/v1/conversations/import?source="+source,{method:"POST",body:await file.arrayBuffer()});
   const data=await res.json();
   if(!res.ok){setError(data.message??"导入被拒绝");return;}
   setNote("已提交导入，后台处理中："+data.id);
   const timer=setInterval(async()=>{
    try{const row=await request<ImportRow>("/api/v1/imports/"+data.id);
     if(row.status!=="pending"&&row.status!=="processing"){clearInterval(timer);setNote(`导入${row.status==="completed"?"完成":"失败"}：新建 ${row.conversations_created}，去重 ${row.conversations_deduplicated}，消息 ${row.messages_imported}`+(row.error?("，"+row.error):""));reload();}
    }catch{clearInterval(timer);}
   },1500);
  }catch(e){setError(String(e));}
 }
 async function exportConv(id:string,format:"archive"|"markdown"){
  setError("");try{
   const res=await requestRaw("/api/v1/conversations/"+id+"/export?format="+format);
   if(!res.ok){setError("导出失败");return;}
   const blob=await res.blob();
   const a=document.createElement("a");a.href=URL.createObjectURL(blob);
   a.download=format==="markdown"?id+".md":id+".archive.json";a.click();URL.revokeObjectURL(a.href);
  }catch(e){setError(String(e));}
 }
 async function removeConv(id:string){
  if(!confirm("删除该会话及其全部分支？"))return;
  setError("");try{await request("/api/v1/conversations/"+id,{method:"DELETE"});if(detail?.id===id)setDetail(null);reload();}catch(e){setError(String(e));}
 }
 async function createProject(){
  if(!projectName.trim())return;
  setError("");try{await request("/api/v1/projects",{method:"POST",body:JSON.stringify({name:projectName})});setProjectName("");reload();}catch(e){setError(String(e));}
 }
 async function assign(convId:string,projectId:string){
  setError("");try{await request("/api/v1/conversations/"+convId,{method:"PATCH",body:JSON.stringify({project_id:projectId||null})});reload();}catch(e){setError(String(e));}
 }

 return <section>
  <p className="eyebrow">CONVERSATION PORTABILITY</p><h1>会话资产</h1>
  <p className="lead">导入外部对话并保留分支与来源，重复导入自动去重；导出为规范 Archive 或 Markdown。</p>
  {error&&<p className="error">{error}</p>}{note&&<p className="muted">{note}</p>}
  <div className="sync-banner"><strong>导入对话</strong>
   <select value={source} onChange={e=>setSource(e.target.value)}>
    <option value="chatgpt_export">ChatGPT 导出 (conversations.json)</option>
    <option value="codex_cli_jsonl">Codex CLI 会话 (JSONL)</option>
    <option value="archive">AI Hub Archive v1</option>
   </select>
   <input type="file" accept=".json,.jsonl,application/json" onChange={e=>{const f=e.target.files?.[0];if(f)void importFile(f);e.target.value="";}}/>
  </div>
  <div className="sync-banner"><strong>新建项目</strong>
   <input value={projectName} onChange={e=>setProjectName(e.target.value)} placeholder="项目名称" maxLength={300}/>
   <button onClick={()=>void createProject()}>创建</button>
  </div>
  {projects.length>0&&<div className="device-list">{projects.map(p=><article className="device-card" key={p.id}><div><h3>{p.name}</h3><p className="muted">{p.conversations} 个会话</p></div></article>)}</div>}
  <div className="device-list">{convs.map(c=><article className="device-card" key={c.id}>
   <div><h3><a href="#" onClick={e=>{e.preventDefault();void open(c.id);}}>{c.title}</a></h3>
    <p className="muted">{c.provider_slug} · {c.message_count} 条消息 · {c.branch_count} 个分支{c.project_name?(" · "+c.project_name):""} · {new Date(c.updated_at).toLocaleString()}</p>
    <p className="muted">归属项目：<select value={detail?.id===c.id?(detail.project_id??""):""} onChange={e=>void assign(c.id,e.target.value)}><option value="">（未归属）</option>{projects.map(p=><option key={p.id} value={p.id}>{p.name}</option>)}</select></p>
   </div>
   <div>
    <button onClick={()=>void exportConv(c.id,"archive")}>导出 Archive</button>
    <button onClick={()=>void exportConv(c.id,"markdown")}>导出 Markdown</button>
    <button onClick={()=>void removeConv(c.id)}>删除</button>
   </div>
  </article>)}</div>
  {imports.length>0&&<><h2>导入记录</h2><div className="device-list">{imports.map(i=><article className="device-card" key={i.id}><div>
   <h3>{i.source_type} · {i.status}</h3><p className="muted">新建 {i.conversations_created} · 去重 {i.conversations_deduplicated} · 消息 {i.messages_imported}{i.error?(" · "+i.error):""}</p>
  </div></article>)}</div></>}
  {detail&&<><h2>{detail.title}</h2><p className="muted">{detail.provider_slug}</p>
   {detail.branches.map(b=><div className="device-list" key={b.id}>
    {b.name!=="main"&&<h3>分支 {b.name}</h3>}
    {b.messages.map(m=><article className="device-card" key={m.id}><div><h3>{m.role}</h3><p className="mono" style={{whiteSpace:"pre-wrap"}}>{m.content}</p></div></article>)}
   </div>)}
  </>}
 </section>;
}
