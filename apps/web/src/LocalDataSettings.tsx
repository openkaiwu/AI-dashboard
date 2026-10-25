import {useEffect, useRef, useState} from 'react';
import {CloudUpload, FolderOpen, RefreshCw} from 'lucide-react';
import {profile} from './session';
import {request} from './api';
import {localTimestamp} from './cacheRefresh';

export type StorageStatus = {auto_sync:boolean;directory:string;last_sync?:string;error?:string;observed_at?:string;queue?:{pending:boolean;attempts:number;next_attempt?:string|null;needs_auth:boolean;last_error?:string|null}};

export default function LocalDataSettings() {
  const [status, setStatus] = useState<StorageStatus|null>(null);
  const [busy, setBusy] = useState(false), [error, setError] = useState('');
  const sequence = useRef(0), operation = useRef(false);
  const id = profile().id;
  useEffect(() => {
    let active = true;
    setStatus(null); setError(''); setBusy(false); operation.current = false;
    const refresh = async () => {
      if (operation.current || document.visibilityState === 'hidden') return;
      const ticket = ++sequence.current;
      try {
        const value = await window.aihubDesktop?.localStatus?.(id);
        if (active && ticket === sequence.current && value) {setStatus(value); setError('');}
      } catch (e) {if (active && ticket === sequence.current) setError(String(e));}
    };
    void refresh();
    const timer = window.setInterval(() => void refresh(), 15000);
    const onVisible = () => {if (document.visibilityState === 'visible') void refresh();};
    document.addEventListener('visibilitychange', onVisible);
    return () => {active = false; sequence.current++; window.clearInterval(timer); document.removeEventListener('visibilitychange', onVisible);};
  }, [id]);
  if (!window.aihubDesktop?.localStatus) return null;

  async function update(auto?: boolean) {
    if (operation.current) return;
    operation.current = true;
    const ticket = ++sequence.current;
    setBusy(true); setError('');
    try {
      if (auto === undefined) await request('/api/v1/me');
      const value = auto === undefined
        ? await window.aihubDesktop!.syncLocalData!(id)
        : await window.aihubDesktop!.localStatus!(id, {auto_sync: auto});
      if (ticket === sequence.current) {
        setStatus(value);
        if (auto === undefined) window.dispatchEvent(new Event('aihub-reminders-changed'));
      }
    } catch (e) {if (ticket === sequence.current) setError(String(e));}
    finally {if (ticket === sequence.current) {operation.current = false; setBusy(false);}}
  }
  const queue = status?.queue;
  return <section className="panel sync-card" aria-label="本地数据与手机同步" aria-busy={busy}>
    <div className="section-heading"><div className="sync-heading"><CloudUpload size={22} aria-hidden="true"/><div><h2>电脑与手机同步</h2><p className="muted compact">额度先保存在此电脑，上传后可在手机查看并提醒。</p></div></div><span className={'sync-state'+(queue?.pending?' pending':'')}>{!status?'读取中':queue?.needs_auth?'需要登录':queue?.pending?'等待上传':status.auto_sync?'自动同步已开启':status.last_sync?'手动同步':'仅本地保存'}</span></div>
    <dl className="sync-facts"><div><dt>数据截至</dt><dd>{localTimestamp(status?.observed_at, '尚未采集')}</dd></div><div><dt>上次成功上传</dt><dd>{localTimestamp(status?.last_sync, '尚未上传')}</dd></div><div><dt>待上传</dt><dd>{!status?'—':queue?.pending?'1 份':'0 份'}</dd></div></dl>
    <div className="sync-actions"><button className="btn" disabled={busy||!status} onClick={()=>void update()}><RefreshCw size={15} className={busy?'spinning':''} aria-hidden="true"/>{busy?'正在处理…':'立即同步到手机'}</button><label className="toggle-row"><input type="checkbox" disabled={busy||!status} checked={status?.auto_sync||false} onChange={e=>void update(e.target.checked)}/>自动同步到手机</label></div>
    {queue?.pending&&!queue.needs_auth&&<p className="muted compact" role="status">待上传数据已保留在本机。{queue.next_attempt?'下次重试：'+localTimestamp(queue.next_attempt):'联网后自动补传。'}</p>}
    {queue?.needs_auth&&<p className="error" role="alert">自动上传已暂停：登录已过期，请重新登录后手动同步。</p>}
    {(error||status?.error)&&<p className="error" role="alert">同步未完成，本地数据已保留：{error||status?.error}</p>}
    <details className="sync-details"><summary><FolderOpen size={15} aria-hidden="true"/>保存位置与卸载保留</summary><p className="mono">{status?.directory||'正在读取保存位置…'}</p><p className="muted compact">数据与安装目录分开保存，覆盖安装不会清空。卸载默认保留；仅在卸载时明确选择删除后清除。</p></details>
  </section>;
}
