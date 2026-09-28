import { FormEvent, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, ApiError } from "../api";
import { getSession, profile, profiles, saveProfile, selectProfile, saveSession, removeProfile } from "../session";
export default function Login(){
 const nav=useNavigate();
 const[email,setEmail]=useState("");const[password,setPassword]=useState("");const[device,setDevice]=useState(window.aihubDesktop?"我的电脑":"管理浏览器");
 const[error,setError]=useState("");const[busy,setBusy]=useState(false);const[showProfile,setShowProfile]=useState(false);
 const[editID,setEditID]=useState<string|undefined>();
 const[url,setURL]=useState("https://");const[name,setName]=useState("");
 async function submit(e:FormEvent){e.preventDefault();setBusy(true);setError("");try{
 const s=await api.login(email,password,device);saveSession(s);nav(window.aihubDesktop?"/choose":"/admin");
 }catch(e){setError(e instanceof ApiError?e.message:"无法连接服务器，请检查地址与网络");}finally{setBusy(false);}}
 return <div className="auth-shell"><div className="auth-intro"><p className="eyebrow">AI HUB / YOUR PERSONAL CONTROL ROOM</p><div className="auth-orbit">↗</div><h1>你的 AI 工作，<br/>在每一端接续。</h1><p className="lead">服务资产一处掌握。电脑与手机跨网络同步，<br/>数据留在你选择的服务器。</p><div className="auth-features"><span>01 / 自部署</span><span>02 / 离线优先</span><span>03 / 跨设备</span></div></div>
 <div className="auth-card"><p className="eyebrow">WELCOME TO YOUR HUB</p><h2>连接你的工作空间</h2><p className="muted">账户由管理员授权；手机和电脑使用同一账户。</p>
 <label className="field"><span>服务器</span><select value={profile().id} onChange={e=>selectProfile(e.target.value)}>{profiles().map(p=><option key={p.id} value={p.id}>{p.name} · {p.url}</option>)}</select></label>
 <div className="actions"><button className="btn ghost" onClick={()=>{setEditID(undefined);setName("");setURL("https://");setShowProfile(!showProfile);}}>＋ 添加服务器</button><button className="btn ghost" onClick={()=>{setEditID(profile().id);setName(profile().name);setURL(profile().url);setShowProfile(true);}}>编辑服务器</button>{profiles().length>1&&<button className="btn ghost" onClick={()=>{try{removeProfile(profile().id);location.reload();}catch(e){setError(String(e));}}}>移除此服务器配置</button>}</div>
 {showProfile&&<form className="profile-form" onSubmit={e=>{e.preventDefault();try{const p=saveProfile(name,url,editID);selectProfile(p.id);}catch(e){setError(String(e));}}}><label className="field"><span>名称</span><input required value={name} onChange={e=>setName(e.target.value)}/></label><label className="field"><span>HTTPS 地址</span><input required value={url} onChange={e=>setURL(e.target.value)}/></label><button className="btn secondary">保存服务器</button></form>}
 {error&&<p className="error" role="alert">{error}</p>}
 <form onSubmit={submit}><label className="field"><span>邮箱</span><input type="email" required autoComplete="email" value={email} onChange={e=>setEmail(e.target.value)}/></label><label className="field"><span>密码</span><input type="password" required minLength={8} autoComplete="current-password" value={password} onChange={e=>setPassword(e.target.value)}/></label><label className="field"><span>此设备的名称</span><input required maxLength={40} value={device} onChange={e=>setDevice(e.target.value)}/></label><div className="actions"><button className="btn" disabled={busy}>{busy?"正在连接…":"进入工作空间 ↗"}</button></div></form>
 {getSession()&&<button className="btn ghost" onClick={()=>nav("/")}>离线打开本地工作空间</button>}
 </div></div>;
}
