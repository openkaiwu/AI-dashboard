import { FormEvent, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { Account, Snapshot, api } from "../api";
import { fmtTime, remainText, sourceLabel, statusLabel } from "./Dashboard";
import { toRFC } from "./AccountNew";

export default function AccountDetail() {
  const { id } = useParams();
  const nav = useNavigate();
  const [account, setAccount] = useState<Account | null>(null);
  const [snaps, setSnaps] = useState<Snapshot[]>([]);
  const [days, setDays] = useState(30);
  const [error, setError] = useState("");
  const [note, setNote] = useState("");
  const [remaining, setRemaining] = useState("");
  const [limit, setLimit] = useState("");
  const [resetAt, setResetAt] = useState("");
  const [expiresAt, setExpiresAt] = useState("");

  async function load() {
    if (!id) return;
    const a = await api.getAccount(id);
    setAccount(a);
    const b = a.buckets[0];
    if (b) {
      const hist = await api.snapshots(b.id, days);
      setSnaps(hist.snapshots);
      setRemaining(b.remaining_value != null ? String(b.remaining_value) : "");
      setLimit(b.limit_value != null ? String(b.limit_value) : "");
    }
  }

  useEffect(() => {
    load().catch((e) => setError(e.message));
  }, [id, days]);

  async function onRefresh(e: FormEvent) {
    e.preventDefault();
    const b = account?.buckets[0];
    if (!b) return;
    try {
      await api.manualSnapshot(b.id, {
        remaining_value: remaining === "" ? null : Number(remaining),
        limit_value: limit === "" ? null : Number(limit),
        reset_at: toRFC(resetAt),
        expires_at: toRFC(expiresAt),
        note,
      });
      setNote("");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "刷新失败");
    }
  }

  if (!account) return error ? <p className="error">{error}</p> : <p className="lead">加载中…</p>;
  const b = account.buckets[0];

  return (
    <>
      <div className="top">
        <div>
          <p className="eyebrow">{account.provider.display_name}</p>
          <h1>{account.display_name}</h1>
          <p>
            {account.plan_name || "未命名套餐"} ·
            <span className={`badge s-${account.computed_status}`} style={{ marginLeft: 8 }}>{statusLabel(account.computed_status)}</span>
          </p>
        </div>
        <div className="actions">
          <button className="btn danger" onClick={async () => {
            if (!confirm("删除该账户及其额度记录？")) return;
            await api.deleteAccount(account.id);
            nav("/");
          }}>删除</button>
        </div>
      </div>
      {error ? <p className="error">{error}</p> : null}

      <div className="panel">
        <h2>当前快照</h2>
        <p className="metric"><strong className="mono">{remainText(account)}</strong></p>
        <p className="meta">
          <span>来源 {b ? sourceLabel(b.source_type) : "无"}</span>
          <span>置信度 {b?.confidence ?? "—"}</span>
          <span>更新 {fmtTime(b?.observed_at ?? null)}</span>
          <span>重置 {fmtTime(b?.reset_at ?? null)}</span>
          <span>过期 {fmtTime(b?.expires_at ?? account.expires_at)}</span>
        </p>
        {b?.note ? <p className="lead">{b.note}</p> : null}
      </div>

      <div className="panel">
        <h2>历史趋势 · 最近 {days} 天</h2>
        <div className="actions" style={{ marginBottom: 12 }}>
          <button className="btn secondary" onClick={() => setDays(7)}>7 天</button>
          <button className="btn secondary" onClick={() => setDays(30)}>30 天</button>
        </div>
        <Sparkline points={snaps.map((s) => s.remaining_ratio ?? 0)} />
        <table className="table">
          <thead><tr><th>时间</th><th>剩余</th><th>比例</th><th>来源</th><th>备注</th></tr></thead>
          <tbody>
            {snaps.slice().reverse().map((s) => (
              <tr key={s.id}>
                <td>{fmtTime(s.observed_at)}</td>
                <td className="mono">{s.remaining_value ?? "—"}</td>
                <td className="mono">{s.remaining_ratio == null ? "—" : `${Math.round(s.remaining_ratio * 100)}%`}</td>
                <td>{sourceLabel(s.source_type)}</td>
                <td>{s.note}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {b ? (
        <form className="panel" onSubmit={onRefresh}>
          <h2>手工刷新</h2>
          <div className="row">
            <label className="field"><span>剩余额度</span><input value={remaining} onChange={(e) => setRemaining(e.target.value)} /></label>
            <label className="field"><span>总额度</span><input value={limit} onChange={(e) => setLimit(e.target.value)} /></label>
          </div>
          <div className="row">
            <label className="field"><span>重置时间</span><input type="datetime-local" value={resetAt} onChange={(e) => setResetAt(e.target.value)} /></label>
            <label className="field"><span>过期时间</span><input type="datetime-local" value={expiresAt} onChange={(e) => setExpiresAt(e.target.value)} /></label>
          </div>
          <label className="field"><span>刷新备注</span><input value={note} onChange={(e) => setNote(e.target.value)} /></label>
          <button className="btn">写入快照并重新求值</button>
        </form>
      ) : null}
    </>
  );
}

function Sparkline({ points }: { points: number[] }) {
  const w = 640, h = 84, p = 6;
  if (points.length === 0) return <p className="meta">还没有历史点。</p>;
  const max = 1;
  const step = points.length === 1 ? 0 : (w - p * 2) / (points.length - 1);
  const d = points.map((v, i) => {
    const x = p + i * step;
    const y = h - p - Math.max(0, Math.min(max, v)) * (h - p * 2);
    return `${i === 0 ? "M" : "L"}${x} ${y}`;
  }).join(" ");
  return (
    <svg className="spark" viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none">
      <path d={d} fill="none" stroke="#c6f25e" strokeWidth="2" />
    </svg>
  );
}
