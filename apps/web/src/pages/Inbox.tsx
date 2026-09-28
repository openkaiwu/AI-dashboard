import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { NotificationItem, api } from "../api";
import { fmtTime } from "./Dashboard";
import { ActiveProvider, getActiveProvider, setActiveProvider } from "../providerMode";

function modeForSlug(slug?: string): ActiveProvider | null {
  if (slug === "cursor") return "cursor";
  if (slug === "codex") return "codex";
  return null;
}

export default function Inbox() {
  const nav = useNavigate();
  const mode = getActiveProvider();
  const [items, setItems] = useState<NotificationItem[]>([]);
  const [error, setError] = useState("");

  async function load() {
    const r = await api.notifications();
    const filtered =
      mode === "cursor"
        ? r.notifications.filter((n) => !n.provider_slug || n.provider_slug === "cursor")
        : r.notifications.filter((n) => !n.provider_slug || n.provider_slug !== "cursor");
    setItems(filtered);
  }

  async function openItem(n: NotificationItem) {
    const target = modeForSlug(n.provider_slug);
    if (target && target !== getActiveProvider()) {
      setActiveProvider(target);
      window.dispatchEvent(new CustomEvent("aihub-provider-mode", { detail: target }));
    }
    if (n.provider_account_id) {
      nav("/accounts/" + encodeURIComponent(n.provider_account_id));
      return;
    }
    if (target === "codex") nav("/codex");
    else nav("/dashboard");
  }

  useEffect(() => {
    load().catch((e) => setError(e.message));
  }, [mode]);

  return (
    <>
      <div className="top">
        <div>
          <p className="eyebrow">Inbox</p>
          <h1>通知中心</h1>
          <p>同一提醒在条件解除前不会重复轰炸。</p>
        </div>
        <button className="btn secondary" onClick={async () => { await api.readAll(); await load(); }}>全部标为已读</button>
      </div>
      {error ? <p className="error">{error}</p> : null}
      {items.length === 0 ? <div className="empty">还没有通知。</div> : (
        <div className="panel" style={{ padding: 0 }}>
          <table className="table">
            <thead><tr><th>时间</th><th>平台</th><th>级别</th><th>内容</th><th></th></tr></thead>
            <tbody>
              {items.map((n) => (
                <tr key={n.id} style={{ opacity: n.status === "read" ? 0.55 : 1 }}>
                  <td>{fmtTime(n.created_at)}</td>
                  <td>{n.provider_name || "—"}</td>
                  <td>{n.severity}</td>
                  <td>
                    <button className="btn ghost" style={{ textAlign: "left" }} onClick={() => void openItem(n)}>
                      <b>{n.title}</b>
                      <div className="meta">{n.body}</div>
                    </button>
                  </td>
                  <td>
                    {n.status === "unread" ? (
                      <span className="actions"><button className="btn ghost" onClick={async () => { await api.notificationAction(n.id,"read"); await load(); }}>已读</button><button className="btn ghost" onClick={async () => { await api.notificationAction(n.id,"snooze"); await load(); }}>稍后</button><button className="btn ghost" onClick={async () => { await api.notificationAction(n.id,"dismiss"); await load(); }}>忽略</button></span>
                    ) : n.status === "snoozed" ? "稍后提醒" : n.status === "dismissed" ? "已忽略" : "已读"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
