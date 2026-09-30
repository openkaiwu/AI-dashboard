import {FormEvent,useEffect,useState} from "react";
import {request} from "../api";
import {saveSession} from "../session";
type User={id:string;email:string;role:string;status:string;created_at:string};
type Device={id:string;name:string;kind:string;revoked_at:string|null};
type Connector={slug:string;bridges:number;last_receive:string|null};
type FailureClass={cls:string;count:number;note:string};
type Report={generated_at:string;connectors:Connector[]|null;freshness:{buckets:number;fresh:number;stale:number};taxonomy:FailureClass[]|null;volume:Record<string,number>|null};
type Operations={database:{reachable:boolean;migrations_applied:number;latest_migration:string};jobs:{pending:number;retried:number};accounts:{users:number;devices:number};backup:{configured:boolean;note:string}};
export default function Admin(){
 const[users,setUsers]=useState<User[]>([]),[devices,setDevices]=useState<Record<string,Device[]>>({}),[email,setEmail]=useState(""),[password,setPassword]=useState(""),[error,setError]=useState(""),[busy,setBusy]=useState(false);
 const[report,setReport]=useState<Report|null>(null),[ops,setOps]=useState<Operations|null>(null);
 async function reload(){try{const data=await request<{users:User[]}>("/api/v1/admin/users");setUsers(data.users);setError("");}catch(e){setError(String(e));}
  try{setReport(await request<Report>("/api/v1/admin/telemetry"));}catch{}
  try{setOps(await request<Operations>("/api/v1/admin/operations"));}catch{}
 }
 useEffect(()=>{void reload();},[]);
 async function create(e:FormEvent){e.preventDefault();setBusy(true);try{await request("/api/v1/admin/users",{method:"POST",body:JSON.stringify({email,password})});setEmail("");setPassword("");await reload();}catch(e){setError(String(e));}finally{setBusy(false);}}
 async function toggle(u:User){try{await request(`/api/v1/admin/users/${u.id}/status`,{method:"PATCH",body:JSON.stringify({status:u.status==='active'?'disabled':'active'})});await reload();}catch(e){setError(String(e));}}
 async function reset(u:User){const next=prompt(`为 ${u.email} 设置新密码（8–72 字节）`);if(!next)return;try{await request(`/api/v1/admin/users/${u.id}/password`,{method:"POST",body:JSON.stringify({password:next})});}catch(e){setError(String(e));}}
 async function openDevices(u:User){try{const data=await request<{devices:Device[]}>(`/api/v1/admin/users/${u.id}/devices`);setDevices(old=>({...old,[u.id]:data.devices}));}catch(e){setError(String(e));}}
 async function unbind(u:User,d:Device){if(!confirm(`解绑 ${u.email} 的 ${d.name}？该设备的会话和采集连接会立即失效。`))return;try{await request(`/api/v1/admin/users/${u.id}/devices/${d.id}`,{method:"DELETE"});await openDevices(u);}catch(e){setError(String(e));}}
 return <section className="panel"><p className="eyebrow">AI HUB ADMIN</p><h1>账户授权与设备绑定</h1><p>账户由管理员创建。每个账户只能绑定一台电脑和一台手机。</p>{error&&<p className="error" role="alert">{error}</p>}
 <form onSubmit={e=>void create(e)}><h2>创建账户</h2><label className="field"><span>邮箱</span><input type="email" required value={email} onChange={e=>setEmail(e.target.value)}/></label><label className="field"><span>初始密码</span><input type="password" minLength={8} required value={password} onChange={e=>setPassword(e.target.value)}/></label><button className="btn" disabled={busy}>创建并授权</button></form>
 <h2>账户</h2>{users.map(u=><article className="device-card" key={u.id}><div><strong>{u.email}</strong><p className="muted">{u.role==='admin'?'管理员':'成员'} · {u.status==='active'?'已授权':u.status==='pending'?'待确认':'已停用'}</p><div className="actions"><button className="btn secondary" onClick={()=>void openDevices(u)}>查看设备</button><button className="btn secondary" onClick={()=>void reset(u)}>重置密码</button><button className="btn ghost" disabled={u.role==='admin'} onClick={()=>void toggle(u)}>{u.status==='active'?'停用':'授权'}</button></div>{devices[u.id]?.map(d=><p key={d.id}>{d.kind==='desktop'?'电脑':d.kind==='mobile'?'手机':d.kind==='web_admin'?'管理浏览器':'旧设备'} · {d.name} · {d.revoked_at?'已解绑':'已绑定'} <button className="btn ghost" disabled={!!d.revoked_at} onClick={()=>void unbind(u,d)}>解绑</button></p>)}</div></article>)}
 <h2>G1 观察报表</h2>{report&&<div className="device-card">
  <p className="muted">生成于 {new Date(report.generated_at).toLocaleString()} · 额度桶 {report.freshness.buckets} 个（新鲜 {report.freshness.fresh} / 过期 {report.freshness.stale}）</p>
  <p className="muted">采集线：{(report.connectors??[]).map(c=>`${c.slug}（${c.last_receive?new Date(c.last_receive).toLocaleString():"无数据"}）`).join(" · ")||"暂无"}</p>
  <p className="muted">分类失败：{(report.taxonomy??[]).map(t=>`${t.cls}×${t.count}`).join(" · ")||"无"}</p>
  <p className="muted">量级：快照 {report.volume?.usage_snapshots??0} · 通知 {report.volume?.notifications_sent??0}（重复 {report.volume?.notification_dupes??0}）· 导入成功/失败 {report.volume?.imports_completed??0}/{report.volume?.imports_failed??0} · 优惠 {report.volume?.promotions??0} · 共享 {report.volume?.resource_shares??0}</p>
 </div>}
 <h2>运维状态</h2>{ops&&<div className="device-card">
  <p className="muted">数据库 {ops.database.reachable?"正常":"不可达"} · 迁移 {ops.database.migrations_applied} 个（最新 {ops.database.latest_migration||"无"}）</p>
  <p className="muted">任务队列：待处理 {ops.jobs.pending} · 重试中 {ops.jobs.retried} · 账户 {ops.accounts.users} / 设备 {ops.accounts.devices}</p>
  <p className="muted">{ops.backup.note}</p>
 </div>}
 <button className="btn ghost" onClick={()=>{saveSession(null);location.assign('/login');}}>退出管理台</button></section>;
}
