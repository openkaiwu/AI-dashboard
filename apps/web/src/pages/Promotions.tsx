import {useEffect,useState} from "react";
import {request} from "../api";

type Promo={id:string;title:string;url:string;provider_slug:string;plan:string;discount:string;ends_at:string|null;time_precision:string;confidence:string;status:string;source:string;created_at:string};
type Watch={id:string;provider_slug:string;plan:string;region:string};
type Source={id:string;slug:string;kind:string;trusted:boolean;enabled:boolean};

export default function Promotions(){
 const[feed,setFeed]=useState<Promo[]>([]),[status,setStatus]=useState("active"),[watchlist,setWatchlist]=useState<Watch[]>([]);
 const[sources,setSources]=useState<Source[]>([]),[error,setError]=useState(""),[note,setNote]=useState("");
 const[url,setUrl]=useState(""),[title,setTitle]=useState(""),[provider,setProvider]=useState(""),[discount,setDiscount]=useState("");
 const[endsAt,setEndsAt]=useState(""),[precision,setPrecision]=useState("unknown");
 const[wProvider,setWProvider]=useState(""),[wPlan,setWPlan]=useState(""),[wRegion,setWRegion]=useState("");
 const[srcSlug,setSrcSlug]=useState(""),[srcKind,setSrcKind]=useState("rss"),[srcUrl,setSrcUrl]=useState("");

 const reload=()=>{void request<{promotions:Promo[]}>(`/api/v1/promotions?status=${status}`).then(x=>setFeed(x.promotions)).catch(e=>setError(String(e)));
  void request<{watchlist:Watch[]}>("/api/v1/promotion-watchlist").then(x=>setWatchlist(x.watchlist)).catch(()=>{});
  void request<{sources:Source[]}>("/api/v1/promotion-sources").then(x=>setSources(x.sources)).catch(()=>{});};
 useEffect(reload,[status]);

 async function submit(){
  setError("");setNote("");
  const body:Record<string,string>={url,title,provider_slug:provider,discount,time_precision:precision};
  if(endsAt)body.ends_at=new Date(endsAt).toISOString();
  try{const out=await request<{created:boolean;notified:number}>("/api/v1/promotions/submit",{method:"POST",body:JSON.stringify(body)});
   setNote(out.created?`已收录，匹配到 ${out.notified} 位订阅者`:"该活动已存在，未重复通知");setTitle("");setUrl("");reload();
  }catch(e){setError(String(e));}
 }
 async function addWatch(){if(!wProvider.trim()&&!wPlan.trim()&&!wRegion.trim()){setError("至少填写一个订阅条件");return;}
  try{await request("/api/v1/promotion-watchlist",{method:"POST",body:JSON.stringify({provider_slug:wProvider,plan:wPlan,region:wRegion})});setWProvider("");setWPlan("");setWRegion("");reload();}catch(e){setError(String(e));}}
 async function removeWatch(id:string){try{await request("/api/v1/promotion-watchlist/"+id,{method:"DELETE"});reload();}catch(e){setError(String(e));}}
 async function createSource(){try{await request("/api/v1/promotion-sources",{method:"POST",body:JSON.stringify({slug:srcSlug,kind:srcKind,url:srcUrl})});setSrcSlug("");setSrcUrl("");reload();}catch(e){setError(String(e));}}
 async function ingest(id:string){try{const out=await request<{created:number;observed:number}>(`/api/v1/promotion-sources/${id}/ingest`,{method:"POST"});setNote(`拉取完成：新增 ${out.created}，去重 ${out.observed}`);reload();}catch(e){setError(String(e));}}

 return <section>
  <p className="eyebrow">PROMOTION INTELLIGENCE</p><h1>优惠情报</h1>
  <p className="lead">官方来源与用户提交统一去重：同一活动不会重复通知；未知结束时间不伪造精确值。</p>
  {error&&<p className="error">{error}</p>}{note&&<p className="muted">{note}</p>}
  <h2>提交优惠</h2>
  <div className="sync-banner"><input value={url} onChange={e=>setUrl(e.target.value)} placeholder="链接（自动去除追踪参数）"/>
   <input value={title} onChange={e=>setTitle(e.target.value)} placeholder="标题"/><input value={provider} onChange={e=>setProvider(e.target.value)} placeholder="Provider（可留空）"/>
   <input value={discount} onChange={e=>setDiscount(e.target.value)} placeholder="优惠内容"/>
   <select value={precision} onChange={e=>setPrecision(e.target.value)}><option value="unknown">结束时间未知</option><option value="day">仅知道日期</option><option value="exact">精确时间</option></select>
   {precision!=="unknown"&&<input type="datetime-local" value={endsAt} onChange={e=>setEndsAt(e.target.value)}/>}
   <button onClick={()=>void submit()}>提交</button></div>
  <h2>我的订阅</h2>
  <div className="sync-banner"><input value={wProvider} onChange={e=>setWProvider(e.target.value)} placeholder="Provider"/>
   <input value={wPlan} onChange={e=>setWPlan(e.target.value)} placeholder="套餐"/><input value={wRegion} onChange={e=>setWRegion(e.target.value)} placeholder="地区"/>
   <button onClick={()=>void addWatch()}>添加订阅</button></div>
  <div className="device-list">{watchlist.map(w=><article className="device-card" key={w.id}><div><h3>{w.provider_slug||"任意 Provider"} · {w.plan||"任意套餐"} · {w.region||"任意地区"}</h3></div><div><button onClick={()=>void removeWatch(w.id)}>退订</button></div></article>)}</div>
  <h2>情报流</h2>
  <div className="sync-banner"><select value={status} onChange={e=>setStatus(e.target.value)}><option value="active">进行中</option><option value="expired">已归档</option></select></div>
  <div className="device-list">{feed.map(p=><article className="device-card" key={p.id}><div>
   <h3>{p.title}</h3>
   <p className="muted">{p.provider_slug||"通用"}{p.plan?(` · ${p.plan}`):""}{p.discount?(` · ${p.discount}`):""} · 来源 {p.source} · 可信度 {p.confidence==="high"?"高":p.confidence==="medium"?"中":"低"}</p>
   <p className="muted">结束时间：{p.ends_at?new Date(p.ends_at).toLocaleString()+(p.time_precision==="day"?"（按日）":""):"未知"}</p>
   <p><a href={p.url} target="_blank" rel="noreferrer" className="mono">{p.url}</a></p>
  </div></article>)}</div>
  <h2>来源注册（管理员）</h2>
  <div className="sync-banner"><input value={srcSlug} onChange={e=>setSrcSlug(e.target.value)} placeholder="slug"/>
   <select value={srcKind} onChange={e=>setSrcKind(e.target.value)}><option value="rss">RSS</option><option value="official_blog">官方博客</option><option value="pricing_page">定价页</option><option value="announcement">公告</option></select>
   <input value={srcUrl} onChange={e=>setSrcUrl(e.target.value)} placeholder="源地址"/>
   <button onClick={()=>void createSource()}>注册</button></div>
  <div className="device-list">{sources.map(s=><article className="device-card" key={s.id}><div><h3>{s.slug} · {s.kind}</h3><p className="muted">{s.trusted?"官方可信":"用户提交"} · {s.enabled?"启用":"停用"}</p></div>
   {s.kind==="rss"&&s.enabled&&<div><button onClick={()=>void ingest(s.id)}>立即拉取</button></div>}</article>)}</div>
 </section>;
}
