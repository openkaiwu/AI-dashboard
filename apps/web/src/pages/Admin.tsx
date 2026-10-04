import {FormEvent,useEffect,useState} from "react";
import {request} from "../api";
import {saveSession} from "../session";
type User={id:string;email:string;role:string;status:string;created_at:string};
type Device={id:string;name:string;kind:string;revoked_at:string|null};
type Connector={slug:string;bridges:number;last_receive:string|null};
type FailureClass={cls:string;count:number;note:string};
type Report={generated_at:string;connectors:Connector[]|null;freshness:{buckets:number;fresh:number;stale:number};taxonomy:FailureClass[]|null;volume:Record<string,number>|null};
type Operations={database:{reachable:boolean;migrations_applied:number;latest_migration:string};jobs:{pending:number;retried:number};accounts:{users:number;devices:number;pending_registrations?:number};backup:{configured:boolean;note:string}};
type Invite={id:string;note:string;max_uses:number;use_count:number;status:string;expires_at?:string;created_at:string};
type Registration={id:string;email:string;note:string;status:string;created_at:string;reviewed_at?:string;reject_reason:string;account_status:string;invite_note:string};
export default function Admin(){
 const[users,setUsers]=useState<User[]>([]),[devices,setDevices]=useState<Record<string,Device[]>>({}),[email,setEmail]=useState(""),[password,setPassword]=useState(""),[error,setError]=useState(""),[busy,setBusy]=useState(false);
 const[report,setReport]=useState<Report|null>(null),[ops,setOps]=useState<Operations|null>(null);
 const[regs,setRegs]=useState<Registration[]>([]),[invites,setInvites]=useState<Invite[]>([]),[newCode,setNewCode]=useState("");
	const[inviteNote,setInviteNote]=useState(""),[inviteDays,setInviteDays]=useState(""),[inviteMaxUses,setInviteMaxUses]=useState(""),[inviteBusy,setInviteBusy]=useState(false);
	const[radarInfo,setRadarInfo]=useState<{empty?:boolean;created_at?:string;note?:string}|null>(null),[radarToken,setRadarToken]=useState(""),[radarNote,setRadarNote]=useState(""),[radarBusy,setRadarBusy]=useState(false);
 async function reload(){try{const data=await request<{users:User[]}>("/api/v1/admin/users");setUsers(data.users);setError("");}catch(e){setError(String(e));}
  try{setReport(await request<Report>("/api/v1/admin/telemetry"));}catch{}
  try{setOps(await request<Operations>("/api/v1/admin/operations"));}catch{}
  try{setRegs((await request<{registrations:Registration[]}>("/api/v1/admin/registrations")).registrations);}catch{}
  try{setInvites((await request<{invites:Invite[]}>("/api/v1/admin/invites")).invites);}catch{}
  try{setRadarInfo(await request<{empty?:boolean;created_at?:string;note?:string}>("/api/v1/admin/radar-token"));}catch{}
 }
 useEffect(()=>{void reload();},[]);
 async function create(e:FormEvent){e.preventDefault();setBusy(true);try{await request("/api/v1/admin/users",{method:"POST",body:JSON.stringify({email,password})});setEmail("");setPassword("");await reload();}catch(e){setError(String(e));}finally{setBusy(false);}}
 async function toggle(u:User){try{await request(`/api/v1/admin/users/${u.id}/status`,{method:"PATCH",body:JSON.stringify({status:u.status==='active'?'disabled':'active'})});await reload();}catch(e){setError(String(e));}}
 async function reset(u:User){const next=prompt(`为 ${u.email} 设置新密码（8–72 字节）`);if(!next)return;try{await request(`/api/v1/admin/users/${u.id}/password`,{method:"POST",body:JSON.stringify({password:next})});}catch(e){setError(String(e));}}
 async function openDevices(u:User){try{const data=await request<{devices:Device[]}>(`/api/v1/admin/users/${u.id}/devices`);setDevices(old=>({...old,[u.id]:data.devices}));}catch(e){setError(String(e));}}
 async function unbind(u:User,d:Device){if(!confirm(`解绑 ${u.email} 的 ${d.name}？该设备的会话和采集连接会立即失效。`))return;try{await request(`/api/v1/admin/users/${u.id}/devices/${d.id}`,{method:"DELETE"});await openDevices(u);}catch(e){setError(String(e));}}
 async function createInvite(e:FormEvent){e.preventDefault();setInviteBusy(true);try{const data=await request<{code:string}>("/api/v1/admin/invites",{method:"POST",body:JSON.stringify({note:inviteNote,days:Number(inviteDays||0),max_uses:Number(inviteMaxUses||1)})});setNewCode(data.code);setInviteNote("");setInviteDays("");setInviteMaxUses("");await reload();}catch(e){setError(String(e));}finally{setInviteBusy(false);}}
 async function toggleInvite(v:Invite){try{await request(`/api/v1/admin/invites/${v.id}/status`,{method:"PATCH",body:JSON.stringify({status:v.status==='active'?'revoked':'active'})});await reload();}catch(e){setError(String(e));}}
 async function rotateRadar(e:FormEvent){e.preventDefault();setRadarBusy(true);try{const data=await request<{token:string}>("/api/v1/admin/radar-token",{method:"POST",body:JSON.stringify({note:radarNote})});setRadarToken(data.token);setRadarNote("");await reload();}catch(e){setError(String(e));}finally{setRadarBusy(false);}}
 async function revokeRadar(){if(!confirm("吊销雷达令牌？自动化任务将立即无法直传检查结果。"))return;try{await request("/api/v1/admin/radar-token",{method:"DELETE"});setRadarToken("");await reload();}catch(e){setError(String(e));}}
 async function approveReg(r:Registration){if(!confirm(`批准 ${r.email} 的注册申请？批准后该用户即可登录。`))return;try{await request(`/api/v1/admin/registrations/${r.id}/approve`,{method:"POST",body:JSON.stringify({})});await reload();}catch(e){setError(String(e));}}
 async function rejectReg(r:Registration){const reason=prompt(`拒绝 ${r.email} 的注册申请，请填写原因（将留存审计）：`);if(!reason)return;try{await request(`/api/v1/admin/registrations/${r.id}/reject`,{method:"POST",body:JSON.stringify({reason})});await reload();}catch(e){setError(String(e));}}
 return <section className="panel"><p className="eyebrow">AI HUB ADMIN</p><h1>账户授权与设备绑定</h1><p>账户由管理员创建。每个账户只能绑定一台电脑和一台手机。</p>{error&&<p className="error" role="alert">{error}</p>}
 <form onSubmit={e=>void create(e)}><h2>创建账户</h2><label className="field"><span>邮箱</span><input type="email" required value={email} onChange={e=>setEmail(e.target.value)}/></label><label className="field"><span>初始密码</span><input type="password" minLength={8} required value={password} onChange={e=>setPassword(e.target.value)}/></label><button className="btn" disabled={busy}>创建并授权</button></form>
 <h2>注册审核{regs.some(r=>r.status==='pending')?`（${regs.filter(r=>r.status==='pending').length} 待审核）`:""}</h2>
 {regs.length===0&&<p className="muted">暂无注册申请。用户可在桌面端、手机 App 或登录页凭邀请码提交注册。</p>}
 {regs.map(r=><article className="device-card" key={r.id}><div><strong>{r.email}</strong><p className="muted">{r.status==='pending'?'待审核':r.status==='approved'?'已批准':'已拒绝'} · 邀请码备注：{r.invite_note||"无"} · 提交于 {new Date(r.created_at).toLocaleString()}{r.reviewed_at?` · 审核于 ${new Date(r.reviewed_at).toLocaleString()}`:""}</p>
  {r.note&&<p className="muted">留言：{r.note}</p>}
  {r.status==='rejected'&&r.reject_reason&&<p className="muted">拒绝原因：{r.reject_reason}</p>}
  {r.status==='pending'&&<div className="actions"><button className="btn" onClick={()=>void approveReg(r)}>批准</button><button className="btn secondary" onClick={()=>void rejectReg(r)}>拒绝</button></div>}
 </div></article>)}
 <h2>邀请码</h2>
 {newCode&&<div className="device-card"><p><strong>新邀请码（仅显示这一次，请立即复制发给用户）：</strong></p><p><code>{newCode}</code></p><div className="actions"><button className="btn" onClick={()=>void navigator.clipboard?.writeText(newCode)}>复制</button><button className="btn ghost" onClick={()=>setNewCode("")}>关闭</button></div></div>}
 <form onSubmit={e=>void createInvite(e)}><label className="field"><span>备注（发给谁）</span><input maxLength={120} value={inviteNote} onChange={e=>setInviteNote(e.target.value)}/></label><label className="field"><span>有效天数（0 = 永不过期）</span><input type="number" min={0} max={365} value={inviteDays} onChange={e=>setInviteDays(e.target.value)}/></label><label className="field"><span>可用次数（1–100）</span><input type="number" min={1} max={100} value={inviteMaxUses} onChange={e=>setInviteMaxUses(e.target.value)}/></label><button className="btn" disabled={inviteBusy}>生成邀请码</button></form>
 {invites.length===0&&<p className="muted">尚未生成任何邀请码。</p>}
 {invites.map(v=><article className="device-card" key={v.id}><div><p className="muted">{v.note||"无备注"} · 已用 {v.use_count}/{v.max_uses} 次 · {v.expires_at?`过期于 ${new Date(v.expires_at).toLocaleString()}`:"永不过期"} · {v.status==='active'?(v.use_count>=v.max_uses?"已用完":"可用"):"已吊销"} · 生成于 {new Date(v.created_at).toLocaleString()}</p><div className="actions"><button className="btn ghost" disabled={v.status!=='active'&&v.use_count>=v.max_uses} onClick={()=>void toggleInvite(v)}>{v.status==='active'?'吊销':'恢复'}</button></div></div></article>)}
 <h2>重置雷达令牌</h2>
 {radarToken&&<div className="device-card"><p><strong>新雷达令牌（仅显示这一次，请立即配置到自动化任务）：</strong></p><p><code>{radarToken}</code></p><div className="actions"><button className="btn" onClick={()=>void navigator.clipboard?.writeText(radarToken)}>复制</button><button className="btn ghost" onClick={()=>setRadarToken("")}>关闭</button></div></div>}
 <p className="muted">自动化任务携带该令牌调用 <code>POST /api/v1/radar/news</code>（body 为 codex-news.json 的内容），即可把检查结果直传为全员雷达，无需桌面端在线。吊销后立即失效。</p>
 <form onSubmit={e=>void rotateRadar(e)}><label className="field"><span>备注（哪台机器/哪个任务）</span><input maxLength={120} value={radarNote} onChange={e=>setRadarNote(e.target.value)}/></label><button className="btn" disabled={radarBusy}>{radarInfo&&!radarInfo.empty?"轮换雷达令牌":"生成雷达令牌"}</button></form>
 {radarInfo&&!radarInfo.empty&&<article className="device-card"><p className="muted">令牌已配置 · 备注：{radarInfo.note||"无"} · 创建于 {new Date(radarInfo.created_at!).toLocaleString()}</p><div className="actions"><button className="btn ghost" onClick={()=>void revokeRadar()}>吊销令牌</button></div></article>}
 <h2>账户</h2>{users.map(u=><article className="device-card" key={u.id}><div><strong>{u.email}</strong><p className="muted">{u.role==='admin'?'管理员':'成员'} · {u.status==='active'?'已授权':u.status==='pending'?'待确认':'已停用'}</p><div className="actions"><button className="btn secondary" onClick={()=>void openDevices(u)}>查看设备</button><button className="btn secondary" onClick={()=>void reset(u)}>重置密码</button><button className="btn ghost" disabled={u.role==='admin'} onClick={()=>void toggle(u)}>{u.status==='active'?'停用':'授权'}</button></div>{devices[u.id]?.map(d=><p key={d.id}>{d.kind==='desktop'?'电脑':d.kind==='mobile'?'手机':d.kind==='web_admin'?'管理浏览器':'旧设备'} · {d.name} · {d.revoked_at?'已解绑':'已绑定'} <button className="btn ghost" disabled={!!d.revoked_at} onClick={()=>void unbind(u,d)}>解绑</button></p>)}</div></article>)}
 <h2>G1 观察报表</h2>{report&&<div className="device-card">
  <p className="muted">生成于 {new Date(report.generated_at).toLocaleString()} · 额度桶 {report.freshness.buckets} 个（新鲜 {report.freshness.fresh} / 过期 {report.freshness.stale}）</p>
  <p className="muted">采集线：{(report.connectors??[]).map(c=>`${c.slug}（${c.last_receive?new Date(c.last_receive).toLocaleString():"无数据"}）`).join(" · ")||"暂无"}</p>
  <p className="muted">分类失败：{(report.taxonomy??[]).map(t=>`${t.cls}×${t.count}`).join(" · ")||"无"}</p>
  <p className="muted">量级：快照 {report.volume?.usage_snapshots??0} · 通知 {report.volume?.notifications_sent??0}（重复 {report.volume?.notification_dupes??0}）· 导入成功/失败 {report.volume?.imports_completed??0}/{report.volume?.imports_failed??0} · 优惠 {report.volume?.promotions??0} · 共享 {report.volume?.resource_shares??0}</p>
 </div>}
 <h2>运维状态</h2>{ops&&<div className="device-card">
  <p className="muted">数据库 {ops.database.reachable?"正常":"不可达"} · 迁移 {ops.database.migrations_applied} 个（最新 {ops.database.latest_migration||"无"}）</p>
  <p className="muted">任务队列：待处理 {ops.jobs.pending} · 重试中 {ops.jobs.retried} · 账户 {ops.accounts.users} / 设备 {ops.accounts.devices}{ops.accounts.pending_registrations?` / 待审核注册 ${ops.accounts.pending_registrations}`:""}</p>
  <p className="muted">{ops.backup.note}</p>
 </div>}
 <button className="btn ghost" onClick={()=>{saveSession(null);location.assign('/login');}}>退出管理台</button></section>;
}
