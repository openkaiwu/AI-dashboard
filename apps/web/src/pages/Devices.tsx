import { useEffect,useState } from "react";
import { request } from "../api";
import { profile } from "../session";
type Device={id:string;name:string;created_at:string;revoked_at:string|null;current:boolean};
export default function Devices(){
 const[devices,setDevices]=useState<Device[]>([]);const[error,setError]=useState("");
 const reload=()=>request<{devices:Device[]}>("/api/v1/devices").then(x=>setDevices(x.devices)).catch(e=>setError(e.message));
 useEffect(()=>{void reload();},[]);
 async function revoke(d:Device){if(!confirm("撤销 "+d.name+"？该设备将无法继续访问和同步。"))return;try{await request("/api/v1/devices/"+d.id,{method:"DELETE"});await reload();}catch(e){setError(String(e));}}
 return <section><p className="eyebrow">YOUR TRUSTED DEVICES</p><h1>设备与连接</h1><p className="lead">电脑和手机可以在不同网络中，连接同一个 HTTPS 服务器。</p><div className="sync-banner"><strong>{profile().name}</strong><span className="mono">{profile().url}</span></div>{error&&<p className="error">{error}</p>}<div className="device-list">{devices.map(d=><article className="device-card" key={d.id}><div className="device-icon">{d.current?"◉":"▣"}</div><div><h3>{d.name} {d.current&&<span className="device-tag">此设备</span>}</h3><p className="muted">{d.revoked_at?"已撤销":"已授权"} · 添加于 {new Date(d.created_at).toLocaleDateString()}</p></div><button className="btn secondary" disabled={!!d.revoked_at||d.current} onClick={()=>void revoke(d)}>撤销访问</button></article>)}</div><div className="panel"><h2>跨网络连接</h2><p className="muted">在可公网访问的服务器部署 AI Hub，配置域名与 HTTPS。两端添加相同服务器地址，再登录同一账户。此页面显示的是系统设备授权，不会同步第三方平台登录凭据。</p></div></section>;
}
