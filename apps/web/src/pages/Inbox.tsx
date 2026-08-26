import { useEffect, useState } from "react";
import { NotificationItem, api } from "../api";
import { fmtTime } from "./Dashboard";

export default function Inbox() {
  const [items, setItems] = useState<NotificationItem[]>([]);
  const [error, setError] = useState("");

  async function load() {
    const r = await api.notifications();
    setItems(r.notifications);
  }

  useEffect(() => {
    load().catch((e) => setError(e.message));
  }, []);

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
            <thead><tr><th>时间</th><th>级别</th><th>内容</th><th></th></tr></thead>
            <tbody>
              {items.map((n) => (
                <tr key={n.id} style={{ opacity: n.status === "read" ? 0.55 : 1 }}>
                  <td>{fmtTime(n.created_at)}</td>
                  <td>{n.severity}</td>
                  <td>
                    <b>{n.title}</b>
                    <div className="meta">{n.body}</div>
                  </td>
                  <td>
                    {n.status === "unread" ? (
                      <button className="btn ghost" onClick={async () => { await api.readNotification(n.id); await load(); }}>标为已读</button>
                    ) : "已读"}
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
