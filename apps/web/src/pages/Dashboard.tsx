import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Account, api, request } from "../api";
import { getActiveProvider } from "../providerMode";

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
  cursor_api2: "Cursor API",
};

const BUCKET_LABEL: Record<string, string> = {
  cursor_models: "Cursor Models",
  other_models: "Other Models",
  default: "总用量",
};

export function statusLabel(s: string) {
  return STATUS_LABEL[s] ?? s;
}

export function sourceLabel(s: string) {
  return SOURCE_LABEL[s] ?? s;
}

export function bucketUsedPercent(b: Account["buckets"][number]) {
  if (b.collection_status === "stale") return "缓存";
  if (b.collection_status === "unavailable" || b.collection_status === "unknown") return "不可用";
  if (b.remaining_ratio != null) return `${Math.round((1 - b.remaining_ratio) * 100)}% 已用`;
  if (b.remaining_value != null) return String(b.remaining_value);
  return "—";
}

export function remainText(account: Account) {
  const b = account.buckets[0];
  if (!b) return "尚未录入";
  if (b.collection_status === "stale") return "缓存数据";
  if (b.collection_status === "unavailable" || b.collection_status === "unknown") return "不可用";
  if (b.remaining_ratio != null) return `${Math.round((1 - b.remaining_ratio) * 100)}% 已用`;
  if (b.remaining_value != null) return String(b.remaining_value);
  return "—";
}

export function fmtTime(v: string | null | undefined) {
  if (!v) return "未设置";
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return v;
  return d.toLocaleString("zh-CN", { month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

function CursorEmpty({ connected, manualOnly }: { connected: boolean; manualOnly: boolean }) {
  if (manualOnly) {
    return (
      <div className="empty">
        当前只有手工登记的 Cursor 账户。
        <br />
        如需自动采集，请在电脑登录 Cursor 并启动 bridge。
      </div>
    );
  }
  if (!connected) {
    return (
      <div className="empty">
        未连接电脑采集器。
        <br />
        请前往「电脑采集器」生成 bridge.json，并在本机启动 aihub-bridge。
      </div>
    );
  }
  return (
    <div className="empty">
      采集程序已连接，但还没有 Cursor 额度快照。
      <br />
      请确认 Cursor 已登录，并等待下一次采集；若持续失败，看板会显示「不可用」而不是假数字。
    </div>
  );
}

export default function Dashboard() {
  const mode = getActiveProvider();
  const providerSlug = mode === "cursor" ? "cursor" : undefined;
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [counts, setCounts] = useState<Record<string, number>>({});
  const [bridgeConnected, setBridgeConnected] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    Promise.all([
      api.dashboard(),
      mode === "cursor" ? request<{ bridges: { revoked_at: string | null; received_at: string | null }[] }>("/api/v1/codex/bridges").catch(() => ({ bridges: [] })) : Promise.resolve({ bridges: [] }),
    ]).then(([d, bridges]) => {
      const filtered = providerSlug ? d.accounts.filter((a) => a.provider.slug === providerSlug) : d.accounts;
      setAccounts(filtered);
      const nextCounts: Record<string, number> = {};
      for (const a of filtered) {
        nextCounts[a.computed_status] = (nextCounts[a.computed_status] ?? 0) + 1;
      }
      setCounts(nextCounts);
      setBridgeConnected(bridges.bridges.some((b) => !b.revoked_at && b.received_at));
    }).catch((e) => setError(e.message));
  }, [providerSlug, mode]);

  return (
    <>
      <div className="top">
        <div>
          <p className="eyebrow">Status first</p>
          <h1>{mode === "cursor" ? "Cursor 额度看板" : "额度看板"}</h1>
          <p>{mode === "cursor" ? "自动采集或手工登记的 Cursor 额度。采集失败时不会显示臆造数字。" : "按状态而不是平台列表排列。来源和更新时间始终可见。"}</p>
        </div>
        <Link className="btn" to="/accounts/new">登记账户</Link>
      </div>
      {error ? <p className="error">{error}</p> : null}
      <div className="counts">
        {Object.entries(STATUS_LABEL).map(([k, label]) => (
          <span className="chip" key={k}><b>{counts[k] ?? 0}</b>{label}</span>
        ))}
      </div>
      {accounts.length === 0 ? (
        mode === "cursor" ? <CursorEmpty connected={bridgeConnected} manualOnly={false} /> : <div className="empty">还没有账户。先手工登记一个平台额度。</div>
      ) : (
        <div className="grid">
          {accounts.map((a) => {
            const buckets = a.buckets.length ? a.buckets : [];
            const primary = buckets[0];
            const stale = buckets.some((b) => b.collection_status === "stale");
            return (
              <Link key={a.id} className={`card s-${a.computed_status}`} to={`/accounts/${a.id}`}>
                <div className="meta">
                  <span>{a.provider.display_name}</span>
                  <span>{a.plan_name || "未命名套餐"}</span>
                  <span className={`badge s-${a.computed_status}`}>{stale ? "采集失败(缓存)" : statusLabel(a.computed_status)}</span>
                </div>
                <h3>{a.display_name}</h3>
                {buckets.length > 1 ? (
                  <div style={{ display: "grid", gap: 10 }}>
                    {buckets.map((b) => {
                      const used = b.remaining_ratio != null ? Math.max(2, Math.min(100, (1 - b.remaining_ratio) * 100)) : 0;
                      return (
                        <div key={b.id}>
                          <div className="metric">
                            <strong className="mono">{bucketUsedPercent(b)}</strong>
                            <span className="meta">{BUCKET_LABEL[b.scope_key] ?? b.scope_key}</span>
                          </div>
                          <div className="bar"><span style={{ width: `${used}%` }} /></div>
                        </div>
                      );
                    })}
                  </div>
                ) : (
                  <>
                    <div className="metric">
                      <strong className="mono">{remainText(a)}</strong>
                      <span className="meta">{primary ? sourceLabel(primary.source_type) : "无数据"}</span>
                    </div>
                    <div className="bar"><span style={{ width: `${Math.max(2, Math.min(100, (1 - (primary?.remaining_ratio ?? 0)) * 100))}%` }} /></div>
                  </>
                )}
                <p className="meta" style={{ marginTop: 12 }}>
                  <span>重置 {fmtTime(primary?.reset_at ?? null)}</span>
                  <span>过期 {fmtTime(primary?.expires_at ?? a.expires_at)}</span>
                  <span>更新 {fmtTime(primary?.observed_at ?? null)}</span>
                  {primary ? <span>{sourceLabel(primary.source_type)}</span> : null}
                </p>
              </Link>
            );
          })}
        </div>
      )}
    </>
  );
}
