import {useEffect,useState} from "react";
import {request} from "../api";

type Asset={id:string;name:string;kind:string;source_platform:string;latest_version:number;updated_at:string};
type Version={id:string;version:number;content_hash:string;created_by:string;created_at:string;loss_report:{path:string;reason:string;detail?:string}[]};
type Binding={id:string;target_platform:string;target_path:string;status:string};
type Detail={asset:Asset;versions:Version[];latest_content:Record<string,unknown>|null;bindings:Binding[]};
type DiffEntry={op:string;path:string;from?:unknown;to?:unknown};
type Discovery={id:string;path:string;platform:string;secret_keys:string[];detected_format:string;status:string};

export default function ConfigAssets(){
 const[assets,setAssets]=useState<Asset[]>([]),[discoveries,setDiscoveries]=useState<Discovery[]>([]);
 const[detail,setDetail]=useState<Detail|null>(null),[error,setError]=useState(""),[note,setNote]=useState("");
 const[importName,setImportName]=useState(""),[importBody,setImportBody]=useState("");
 const[diff,setDiff]=useState<DiffEntry[]|null>(null),[fromV,setFromV]=useState("1"),[toV,setToV]=useState("");
 const[transformed,setTransformed]=useState("");

 const reload=()=>{void request<{assets:Asset[]}>("/api/v1/config-assets").then(x=>setAssets(x.assets)).catch(e=>setError(String(e)));
  void request<{discoveries:Discovery[]}>("/api/v1/config-discoveries").then(x=>setDiscoveries(x.discoveries)).catch(()=>{});};
 useEffect(reload,[]);

 async function open(id:string){
  setError("");setDiff(null);setTransformed("");setNote("");
  try{const d=await request<Detail>("/api/v1/config-assets/"+id);setDetail(d);setToV(String(d.asset.latest_version));}catch(e){setError(String(e));}
 }
 async function importClaude(){
  setError("");setNote("");
  try{
   JSON.parse(importBody);
   const out=await request<{id:string;loss_report:unknown[]}>("/api/v1/config-assets/import",{method:"POST",body:JSON.stringify({platform:"claude_desktop",name:importName||undefined,content:importBody})});
   setNote(`已导入为配置资产 ${out.id}，记录了 ${out.loss_report.length} 条转换损失（Secret 以名称引用，值未保存）`);
   setImportBody("");setImportName("");reload();
  }catch(e){setError(String(e));}
 }
 async function rollback(version:number){
  setError("");try{const out=await request<{version:number}>("/api/v1/config-assets/"+detail!.asset.id+"/rollback",{method:"POST",body:JSON.stringify({version})});
   setNote(`已回滚：版本 ${version} 的内容已作为新版本 ${out.version} 追加，历史版本保持不变`);await open(detail!.asset.id);reload();}catch(e){setError(String(e));}
 }
 async function runDiff(){
  setError("");try{const out=await request<{diff:DiffEntry[]}>(`/api/v1/config-assets/${detail!.asset.id}/diff?from=${fromV}&to=${toV}`);setDiff(out.diff);}catch(e){setError(String(e));}
 }
 async function previewTransform(){
  setError("");try{const out=await request<{content:string;loss_report:{path:string;reason:string}[]}>("/api/v1/config-assets/"+detail!.asset.id+"/transform",{method:"POST",body:JSON.stringify({target_platform:"codex_cli"})});
   setTransformed(out.content);setNote(`转换完成，${out.loss_report.length} 条损失记录`);}catch(e){setError(String(e));}
 }
 async function removeAsset(id:string){
  if(!confirm("删除该配置资产及全部版本？"))return;
  try{await request("/api/v1/config-assets/"+id,{method:"DELETE"});if(detail?.asset.id===id)setDetail(null);reload();}catch(e){setError(String(e));}
 }

 return <section>
  <p className="eyebrow">PORTABLE CONFIG</p><h1>可移植配置</h1>
  <p className="lead">配置资产以规范形态保存，Secret 一律以名称引用；版本只追加，回滚生成新版本。</p>
  {error&&<p className="error">{error}</p>}{note&&<p className="muted">{note}</p>}
  <div className="sync-banner"><strong>从 Claude Desktop 导入</strong>
   <input value={importName} onChange={e=>setImportName(e.target.value)} placeholder="资产名称（可留空）"/>
   <textarea value={importBody} onChange={e=>setImportBody(e.target.value)} placeholder='粘贴 claude_desktop_config.json 内容' rows={4} style={{minWidth:"24rem"}}/>
   <button onClick={()=>void importClaude()}>导入</button>
  </div>
  <div className="device-list">{assets.map(a=><article className="device-card" key={a.id}>
   <div><h3><a href="#" onClick={e=>{e.preventDefault();void open(a.id);}}>{a.name}</a></h3>
    <p className="muted">{a.kind} · 来源 {a.source_platform} · v{a.latest_version} · {new Date(a.updated_at).toLocaleString()}</p></div>
   <div><button onClick={()=>void removeAsset(a.id)}>删除</button></div>
  </article>)}</div>
  {detail&&<><h2>{detail.asset.name} <span className="muted">v{detail.asset.latest_version}</span></h2>
   <h3>版本历史</h3>
   <div className="device-list">{detail.versions.map(v=><article className="device-card" key={v.id}>
    <div><h3>v{v.version} · {v.created_by}</h3>
     <p className="muted mono">{v.content_hash.slice(0,16)}… · {new Date(v.created_at).toLocaleString()}{v.loss_report.length>0?(` · ${v.loss_report.length} 条损失`):""}</p></div>
    {v.version!==detail.asset.latest_version&&<div><button onClick={()=>void rollback(v.version)}>回滚到此版本</button></div>}
   </article>)}</div>
   <h3>版本对比</h3>
   <div className="sync-banner">
    <select value={fromV} onChange={e=>setFromV(e.target.value)}>{detail.versions.map(v=><option key={v.id} value={v.version}>v{v.version}</option>)}</select>
    <span>→</span>
    <select value={toV} onChange={e=>setToV(e.target.value)}>{detail.versions.map(v=><option key={v.id} value={v.version}>v{v.version}</option>)}</select>
    <button onClick={()=>void runDiff()}>对比</button>
   </div>
   {diff&&<div className="device-list">{diff.length===0?<p className="muted">两个版本内容一致。</p>:diff.map((d,i)=><article className="device-card" key={i}>
    <div><h3>{d.op==="add"?"新增":d.op==="remove"?"移除":"修改"} <span className="mono">{d.path}</span></h3>
     <p className="muted mono">{JSON.stringify(d.from??"")} → {JSON.stringify(d.to??"")}</p></div>
   </article>)}</div>}
   <h3>转换为 Codex CLI (config.toml 片段)</h3>
   <button onClick={()=>void previewTransform()}>生成预览</button>
   {transformed&&<pre className="mono">{transformed}</pre>}
   {detail.bindings.length>0&&<><h3>绑定</h3><div className="device-list">{detail.bindings.map(b=><article className="device-card" key={b.id}><div><h3>{b.target_platform}</h3><p className="muted mono">{b.target_path} · {b.status}</p></div></article>)}</div></>}
  </>}
  {discoveries.length>0&&<><h2>电脑发现的配置</h2><p className="muted">来自桌面 Bridge 的授权目录扫描，只包含文件元数据与 Secret 键名，值从未上传。</p>
   <div className="device-list">{discoveries.map(d=><article className="device-card" key={d.id}><div>
    <h3>{d.platform} · {d.detected_format}</h3><p className="muted mono">{d.path}</p>
    <p className="muted">Secret 键名：{d.secret_keys.length>0?d.secret_keys.join("、"):"（未发现）"}</p>
   </div></article>)}</div></>}
 </section>;
}
