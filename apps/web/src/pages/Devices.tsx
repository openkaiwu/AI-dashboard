import {useEffect,useState} from "react";
import {request} from "../api";
import {profile} from "../session";
type Device={id:string;name:string;kind:string;created_at:string;revoked_at:string|null;current:boolean};
export default function Devices(){
 const[devices,setDevices]=useState<Device[]>([]),[error,setError]=useState("");
 useEffect(()=>{void request<{devices:Device[]}>("/api/v1/devices").then(x=>setDevices(x.devices)).catch(e=>setError(String(e)));},[]);
 return <section><p className="eyebrow">YOUR TRUSTED DEVICES</p><h1>设备与连接</h1><p className="lead">每个账户最多绑定一台电脑和一台手机。更换设备请联系管理员解绑。</p><div className="sync-banner"><strong>{profile().name}</strong><span className="mono">{profile().url}</span></div>{error&&<p className="error">{error}</p>}<div className="device-list">{devices.map(d=><article className="device-card" key={d.id}><div className="device-icon">{d.current?"◉":"▣"}</div><div><h3>{d.name} {d.current&&<span className="device-tag">此设备</span>}</h3><p className="muted">{d.kind==='desktop'?'电脑':d.kind==='mobile'?'手机':'旧设备'} · {d.revoked_at?'已撤销':'已授权'} · 添加于 {new Date(d.created_at).toLocaleDateString()}</p></div></article>)}</div></section>;
}
