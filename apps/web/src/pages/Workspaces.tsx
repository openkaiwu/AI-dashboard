import {useEffect,useState} from "react";
import {request} from "../api";

type Workspace={id:string;name:string;role:string;members:number;owner:boolean;created_at:string};
type Member={user_id:string;email:string;role:string;joined_at:string};
type Invite={id:string;email:string;role:string;status:string};
type MyInvite={id:string;workspace_name:string;role:string};
type Comment={id:string;email:string;body:string;created_at:string};

export default function Workspaces(){
 const[workspaces,setWorkspaces]=useState<Workspace[]>([]),[members,setMembers]=useState<Member[]>([]);
 const[myInvites,setMyInvites]=useState<MyInvite[]>([]),[invites,setInvites]=useState<Invite[]>([]);
 const[selected,setSelected]=useState(""),[error,setError]=useState(""),[note,setNote]=useState("");
 const[name,setName]=useState(""),[inviteEmail,setInviteEmail]=useState(""),[inviteRole,setInviteRole]=useState("editor");
 const[targetID,setTargetID]=useState(""),[comments,setComments]=useState<Comment[]>([]),[commentBody,setCommentBody]=useState("");

 const reload=()=>{void request<{workspaces:Workspace[]}>("/api/v1/workspaces").then(x=>setWorkspaces(x.workspaces)).catch(e=>setError(String(e)));
  void request<{invites:MyInvite[]}>("/api/v1/invites/mine").then(x=>setMyInvites(x.invites)).catch(()=>{});};
 useEffect(reload,[]);

 async function openWorkspace(id:string){
  setSelected(id);setError("");
  try{const x=await request<{members:Member[]}>(`/api/v1/workspaces/${id}/members`);setMembers(x.members);
   const y=await request<{invites:Invite[]}>(`/api/v1/workspaces/${id}/invites`);setInvites(y.invites);}catch{setInvites([]);}
 }
 async function create(){if(!name.trim())return;try{await request("/api/v1/workspaces",{method:"POST",body:JSON.stringify({name})});setName("");reload();}catch(e){setError(String(e));}}
 async function invite(){if(!inviteEmail.trim())return;try{await request(`/api/v1/workspaces/${selected}/invites`,{method:"POST",body:JSON.stringify({email:inviteEmail,role:inviteRole})});setInviteEmail("");openWorkspace(selected);}catch(e){setError(String(e));}}
 async function accept(id:string){try{await request(`/api/v1/invites/${id}/accept`,{method:"POST"});setNote("已加入工作区");reload();}catch(e){setError(String(e));}}
 async function revoke(id:string){try{await request(`/api/v1/invites/${id}`,{method:"DELETE"});openWorkspace(selected);}catch(e){setError(String(e));}}
 async function removeMember(uid:string){if(!confirm("移除该成员后其访问立即失效，确定？"))return;try{await request(`/api/v1/workspaces/${selected}/members/${uid}`,{method:"DELETE"});openWorkspace(selected);}catch(e){setError(String(e));}}
 async function loadComments(){if(!targetID.trim())return;try{const x=await request<{comments:Comment[]}>(`/api/v1/workspace-comments?target_type=conversation&target_id=${targetID}`);setComments(x.comments);}catch(e){setError(String(e));}}
 async function postComment(){if(!targetID.trim()||!commentBody.trim())return;try{await request("/api/v1/workspace-comments",{method:"POST",body:JSON.stringify({target_type:"conversation",target_id:targetID,body:commentBody})});setCommentBody("");loadComments();}catch(e){setError(String(e));}}

 return <section>
  <p className="eyebrow">WORKSPACE &amp; COLLABORATION</p><h1>协作工作区</h1>
  <p className="lead">工作区共享会话与配置资产，不共享任何第三方账号；成员移除后访问立即失效。</p>
  <p className="muted">权限说明：所有者管理成员与邀请 · 编辑者可读可评论 · 查看者仅可读；共享资源的内容只有原始所有者能修改。</p>
  {error&&<p className="error">{error}</p>}{note&&<p className="muted">{note}</p>}
  {myInvites.length>0&&<div className="device-list">{myInvites.map(i=><article className="device-card" key={i.id}><div><h3>邀请：{i.workspace_name}（{i.role==="editor"?"编辑者":"查看者"}）</h3></div><div><button onClick={()=>void accept(i.id)}>接受</button></div></article>)}</div>}
  <div className="sync-banner"><strong>新建工作区</strong><input value={name} onChange={e=>setName(e.target.value)} placeholder="工作区名称"/><button onClick={()=>void create()}>创建</button></div>
  <div className="device-list">{workspaces.map(w=><article className="device-card" key={w.id} onClick={()=>void openWorkspace(w.id)} style={{cursor:"pointer"}}>
   <div><h3>{w.name} <span className="device-tag">{w.role==="owner"?"所有者":w.role==="editor"?"编辑者":"查看者"}</span></h3><p className="muted">{w.members} 名成员</p></div>
  </article>)}</div>
  {selected&&<><h2>成员与邀请</h2>
   <div className="device-list">{members.map(m=><article className="device-card" key={m.user_id}><div><h3>{m.email}</h3><p className="muted">{m.role}</p></div>
    {m.role!=="owner"&&<div><button onClick={()=>void removeMember(m.user_id)}>移除</button></div>}</article>)}</div>
   <h3>邀请新成员</h3>
   <div className="sync-banner"><input value={inviteEmail} onChange={e=>setInviteEmail(e.target.value)} placeholder="对方账户邮箱"/>
    <select value={inviteRole} onChange={e=>setInviteRole(e.target.value)}><option value="editor">编辑者</option><option value="viewer">查看者</option></select>
    <button onClick={()=>void invite()}>邀请</button></div>
   {invites.length>0&&<div className="device-list">{invites.map(i=><article className="device-card" key={i.id}><div><h3>{i.email} · {i.role}</h3><p className="muted">{i.status}</p></div>{i.status==="pending"&&<div><button onClick={()=>void revoke(i.id)}>撤销</button></div>}</article>)}</div>}
   <h3>共享资源评论</h3>
   <div className="sync-banner"><input value={targetID} onChange={e=>setTargetID(e.target.value)} placeholder="会话 ID"/><button onClick={()=>void loadComments()}>查看评论</button></div>
   <div className="device-list">{comments.map(c=><article className="device-card" key={c.id}><div><h3>{c.email}</h3><p className="muted">{c.body} · {new Date(c.created_at).toLocaleString()}</p></div></article>)}</div>
   <div className="sync-banner"><input value={commentBody} onChange={e=>setCommentBody(e.target.value)} placeholder="写下评论"/><button onClick={()=>void postComment()}>发表</button></div>
  </>}
 </section>;
}
