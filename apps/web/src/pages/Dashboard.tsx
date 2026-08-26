import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Account, api } from "../api";

const STATUS_LABEL: Record<string, string> = {
  healthy: "正常",
  low: "额度不足",
  reset_soon_unused: "即将重置",
  expire_soon_unused: "即将过期",
  stale: "数据过期",
  unknown: "未知",
};

const SOURCE_LABEL: Record<string, string> = {
  user_manual: "用户手工",
  official_api: "官方 API",
  official_web_ui: "网页读取",
};

export function statusLabel(s: string) {
  return STATUS_LABEL[s] ?? s;
}

export function sourceLabel(s: string) {
  return SOURCE_LABEL[s] ?? s;
}

export function remainText(account: Account) {
  const b = account.buckets[0];
  if (!b) return "尚未录入";
  if (b.remaining_ratio != null) return `${Math.round(b.remaining_ratio * 100)}%`;
  if (b.remaining_value != null) return String(b.remaining_value);
  return "—";
}

export function fmtTime(v: string | null | undefined) {
  if (!v) return "未设置";
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return v;
  return d.toLocaleString("zh-CN", { month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

export default function Dashboard() {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [counts, setCounts] = useState<Record<string, number>>({});
  const [error, setError] = useState("");

  useEffect(() => {
    api.dashboard().then((d) => {
      setAccounts(d.accounts);
      setCounts(d.counts);
    }).catch((e) => setError(e.message));
  }, []);

  return (
    <>
      <div className="top">
        <div>
          <p className="eyebrow">Status first</p>
          <h1>额度看板</h1>
          <p>按状态而不是平台列表排列。来源和更新时间始终可见。</p>
        </div>
        <Link className="btn" to="/accounts/new">登记账户</Link>
      </div>
      {error ? <p className="error">{error}</p> : null}
      <div className="counts">
        {Object.entries(STATUS_LABEL).map(([k, label]) => (
          <span className="chip" key={k}><b>{counts[k] ?? 0}</b>{label}</span>
        ))}
      </div>
      {accounts.length === 0 ? <div className="empty">还没有账户。先手工登记一个平台额度。</div> : (
        <div className="grid">
          {accounts.map((a) => {
            const b = a.buckets[0];
            const ratio = b?.remaining_ratio ?? 0;
            return (
              <Link key={a.id} className={`card s-${a.computed_status}`} to={`/accounts/${a.id}`}>
                <div className="meta">
                  <span>{a.provider.display_name}</span>
                  <span>{a.plan_name || "未命名套餐"}</span>
                  <span className={`badge s-${a.computed_status}`}>{statusLabel(a.computed_status)}</span>
                </div>
                <h3>{a.display_name}</h3>
                <div className="metric">
                  <strong className="mono">{remainText(a)}</strong>
                  <span className="meta">{b ? sourceLabel(b.source_type) : "无数据"}</span>
                </div>
                <div className="bar"><span style={{ width: `${Math.max(2, Math.min(100, ratio * 100))}%` }} /></div>
                <p className="meta" style={{ marginTop: 12 }}>
                  <span>重置 {fmtTime(b?.reset_at ?? null)}</span>
                  <span>过期 {fmtTime(b?.expires_at ?? a.expires_at)}</span>
                  <span>更新 {fmtTime(b?.observed_at ?? null)}</span>
                </p>
              </Link>
            );
          })}
        </div>
      )}
    </>
  );
}
