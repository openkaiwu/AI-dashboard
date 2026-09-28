import {useEffect,useState} from "react";
import {useNavigate} from "react-router-dom";
import {api,request} from "../api";
import {ActiveProvider,setActiveProvider} from "../providerMode";
type Status={label:string;updated:string};
export default function ModeSelect(){
 const nav=useNavigate();const[status,setStatus]=useState<Record<string,Status>>({});
 useEffect(()=>{let live=true;Promise.allSettled([
   request<{bridges:{received_at:string|null;revoked_at:string|null}[]}>("/api/v1/codex/bridges"),
   api.dashboard(),
 ]).then(([codex,cursor])=>{if(!live)return;const next:Record<string,Status>={};
   if(codex.status==='fulfilled'){const b=codex.value.bridges.find(x=>!x.revoked_at);next.codex={label:b?.received_at?'已连接':'等待电脑连接',updated:b?.received_at||''};}
   if(cursor.status==='fulfilled'){const a=cursor.value.accounts.find(x=>x.provider.slug==='cursor');const b=a?.buckets?.[0];next.cursor={label:a?'已有额度数据':'等待采集或手工录入',updated:b?.observed_at||''};}
   setStatus(next);
 });return()=>{live=false};},[]);
 function choose(mode:ActiveProvider){setActiveProvider(mode);nav(mode==='codex'?'/codex':'/cursor');}
 return <section><p className="eyebrow">AI HUB / WORKSPACE</p><h1>选择工作模式</h1><p className="lead">两种额度都可在电脑后台采集。切换页面不会暂停另一平台的采集与提醒。</p><div className="codex-grid">{(['codex','cursor'] as const).map(mode=><article className="panel codex-card" key={mode}><h2>{mode==='codex'?'Codex':'Cursor'}</h2><p>{status[mode]?.label||'正在读取连接状态'}</p><p className="muted compact">{status[mode]?.updated?'最近更新 '+new Date(status[mode].updated).toLocaleString():'尚无更新时间'}</p><button className="btn" onClick={()=>choose(mode)}>进入 {mode==='codex'?'Codex':'Cursor'} 模式 ↗</button></article>)}</div></section>;
}
