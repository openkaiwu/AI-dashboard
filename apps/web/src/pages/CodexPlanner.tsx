import {useState} from 'react';
import {Link} from 'react-router-dom';
import {budget} from '../quotaBudget';
export default function CodexPlanner({device,stale}:{device:any;stale:boolean}){
 const [reserve,setReserve]=useState(10);
 if(!device)return <p className="panel">连接电脑后可以规划额度。</p>;
 const metrics=device.analysis.metrics;
 const events=[...metrics.filter((m:any)=>m.resets_at).map((m:any)=>({id:m.bucket+m.slot,title:`${m.bucket} · ${m.duration_minutes/60} 小时窗口重置`,at:m.resets_at})),...(device.snapshot.reset_credits?.items||[]).filter((c:any)=>c.expires_at).map((c:any)=>({id:c.id,title:'重置卡到期',at:c.expires_at}))].sort((a,b)=>a.at-b.at);
 function download(){
  const rows=[['采集时间','额度桶','窗口','已用百分比','重置时间']];
  for(const s of device.history)for(const b of s.buckets||[])for(const slot of ['primary','secondary'])if(b[slot])rows.push([s.observed_at,b.id,slot,String(b[slot].used_percent),b[slot].resets_at?new Date(b[slot].resets_at*1000).toISOString():'']);
  const csv=rows.map(r=>r.map((v:string)=>'"'+(/^[=+@\-\t\r]/.test(v)?"'":'')+v.replaceAll('"','""')+'"').join(',')).join('\r\n');
  const url=URL.createObjectURL(new Blob(['\uFEFF'+csv],{type:'text/csv;charset=utf-8'}));const a=document.createElement('a');a.href=url;a.download='codex-quota-history.csv';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);
 }
 return <div className="planner-grid"><section className="panel"><h2>给后续工作留一点空间</h2><p className="muted">按本轮剩余时间分配额度，选择要预留的百分点，比较每个窗口的预算。</p><label className="field"><span>预留额度：{reserve} 个百分点</span><input type="range" min="0" max="50" step="5" value={reserve} onChange={e=>setReserve(Number(e.target.value))}/></label><p className="muted compact">临时试算，不修改提醒规则。预留值指完整额度的百分比。</p>{metrics.map((m:any)=>{const b=budget(m.remaining,m.hours_left,reserve,stale);return <article className="budget-row" key={m.bucket+m.slot}><h3>{m.bucket} · {m.duration_minutes/60} 小时窗口</h3><div className="tile-row"><span>本轮可分配</span><strong>{b?b.available.toFixed(1)+' 个百分点':'等待有效数据'}</strong></div><div className="tile-row"><span>未来 24 小时预算上限</span><strong>{b?b.nextDay.toFixed(1)+' 个百分点':'暂不计算'}</strong></div>{b?.available===0&&<p className="muted">剩余额度已达到预留线，建议等待重置。</p>}</article>;})}<p className="muted compact">预算 = max(剩余额度 − 预留, 0)，按距离重置的时间均摊。多个窗口同时生效，需要分别满足预算；不同任务消耗差异较大。</p><Link to="/notes" className="btn secondary">把工作安排记入跨端便笺 ↗</Link></section><section className="panel"><h2>重置与到期时间线</h2><p className="muted compact">按当前电脑的最近快照排序；时间到达后以新采集结果确认。</p>{stale&&<p className="error">快照已过期，以下仅为最后已知时间。</p>}{events.length?events.map(e=><article className="timeline-row" key={e.id}><span className="status-dot"/><div><h3>{e.title}</h3><time>{new Date(e.at*1000).toLocaleString()}</time>{e.at*1000<=Date.now()&&<p className="muted compact">已到时间，等待采集确认</p>}</div></article>):<p className="muted">尚未返回明确的重置或到期时间。</p>}<div className="export-block"><h3>带走你的使用记录</h3><p className="muted compact">导出当前电脑最近最多 6 小时的已采集额度记录，便于复盘。共 {device.history.length} 次采集，不包含对话或密钥。</p><button className="btn secondary" disabled={!device.history.length} onClick={download}>导出趋势 CSV</button></div></section></div>;
}
