import {useEffect,useState} from 'react';
import {profile} from './session';
import {request} from './api';
export type StorageStatus={auto_sync:boolean;directory:string;last_sync?:string;error?:string;observed_at?:string;queue?:{pending:boolean;attempts:number;next_attempt?:string|null;needs_auth:boolean;last_error?:string|null}};
export default function LocalDataSettings(){
 const [status,setStatus]=useState<StorageStatus|null>(null),[busy,setBusy]=useState(false),[error,setError]=useState('');
 const id=profile().id;
 useEffect(()=>{let active=true;const refresh=()=>{void window.aihubDesktop?.localStatus?.(id).then(value=>{if(active)setStatus(value);}).catch(e=>{if(active)setError(String(e));});};refresh();const timer=window.setInterval(refresh,15000);return()=>{active=false;window.clearInterval(timer);};},[id]);
 if(!window.aihubDesktop?.localStatus)return null;
 async function sync(){setBusy(true);setError('');try{await request('/api/v1/me');setStatus(await window.aihubDesktop!.syncLocalData!(id));window.dispatchEvent(new Event('aihub-reminders-changed'));}catch(e){setError(String(e));}finally{setBusy(false);}}
 async function toggle(value:boolean){try{setStatus(await window.aihubDesktop!.localStatus!(id,{auto_sync:value}));}catch(e){setError(String(e));}}
 const queue=status?.queue;
 return <section className="panel" aria-label="本地数据与手机同步"><h2>本地数据与手机同步</h2><p>额度样本、消耗分析和提醒状态先保存在此电脑。同步后，手机才能收到这些数据与相关提醒。</p><div className="actions"><button className="btn" disabled={busy} onClick={()=>void sync()}>{busy?'正在同步…':'立即同步到手机'}</button><label className="toggle-row"><input type="checkbox" checked={status?.auto_sync||false} onChange={e=>void toggle(e.target.checked)}/>同步到手机并提醒（自动上传额度与统计）</label></div><p className="muted compact">{status?.last_sync?'上次成功同步：'+new Date(status.last_sync).toLocaleString():'尚未同步 · 默认只保存在本地'}{status?.observed_at?' · 最近采集：'+new Date(status.observed_at).toLocaleTimeString():''}{queue?.pending?' · 有 1 份待上传数据':''}{queue?.pending&&queue.next_attempt&&!queue.needs_auth?'，'+(Date.parse(queue.next_attempt)>Date.now()?'下次自动重试 '+new Date(queue.next_attempt).toLocaleTimeString():'即将重试'):''}</p>{queue?.needs_auth&&<p className="error" role="alert">自动上传已暂停：登录已过期，请重新登录后手动同步。</p>}<details><summary>保存位置与卸载保留</summary><p className="mono" style={{overflowWrap:'anywhere'}}>{status?.directory}</p><p>数据与安装目录分开保存，覆盖安装不会清空。卸载默认保留；仅在卸载时明确选择删除后清除。断网时未完成的备份会保留在本机队列，联网后自动补传。</p></details>{(error||status?.error)&&<p className="error" role="alert">同步未完成，本地数据已保留：{error||status?.error}</p>}</section>;
}
