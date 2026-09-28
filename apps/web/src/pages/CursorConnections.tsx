import { useEffect, useState } from "react";
import { request } from "../api";
import { profile, scope } from "../session";
import { CODEX_POLL_MS, CODEX_STALE_MS } from "../codexTiming";

type Bridge = {
  id: string;
  name: string;
  revoked_at: string | null;
  received_at: string | null;
};

export default function CursorConnections() {
  const key = "hub_cursor:" + scope();
  const [bridges, setBridges] = useState<Bridge[]>(() => {
    try {
      return JSON.parse(localStorage.getItem(key) || "[]");
    } catch {
      return [];
    }
  });
  const [error, setError] = useState("");
  const [name, setName] = useState("我的电脑");
  const [busy, setBusy] = useState(false);
  const [token, setToken] = useState("");
  const [tick, setTick] = useState(Date.now());

  async function reload() {
    try {
      const data = await request<{ bridges: Bridge[] }>("/api/v1/codex/bridges");
      localStorage.setItem(key, JSON.stringify(data.bridges));
      setBridges(data.bridges);
      setError("");
    } catch {
      setError("同步暂不可用，显示本机缓存；请检查网络或重新登录。");
    }
  }

  useEffect(() => {
    let active = true;
    let loading = false;
    const controller = new AbortController();
    const update = async () => {
      if (loading) return;
      loading = true;
      try {
        const data = await request<{ bridges: Bridge[] }>("/api/v1/codex/bridges", { signal: controller.signal });
        if (active) {
          localStorage.setItem(key, JSON.stringify(data.bridges));
          setBridges(data.bridges);
          setError("");
        }
      } catch {
        if (active) setError("同步暂不可用，显示本机缓存；请检查网络或重新登录。");
      } finally {
        loading = false;
      }
    };
    void update();
    const timer = setInterval(() => {
      setTick(Date.now());
      void update();
    }, CODEX_POLL_MS);
    return () => {
      active = false;
      controller.abort();
      clearInterval(timer);
    };
  }, [key]);

  async function connect() {
    setBusy(true);
    try {
      const data = await request<{ token: string }>("/api/v1/codex/bridges", { method: "POST", body: JSON.stringify({ name }) });
      setToken(data.token);
      await reload();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  function download() {
    const blob = new Blob(
      [JSON.stringify({ server: profile().url, token, codex_path: "", cursor_enabled: true, interval_seconds: 300 }, null, 2)],
      { type: "application/json" }
    );
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "bridge.json";
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  async function revoke(b: Bridge) {
    if (!confirm("停止 " + b.name + " 的上报？\n\n此连接与 Codex 共用 bridge 令牌，撤销后两边采集都会停止。")) return;
    try {
      await request("/api/v1/codex/bridges/" + encodeURIComponent(b.id), { method: "DELETE" });
      await reload();
    } catch (e) {
      setError(String(e));
    }
  }

  return (
    <section>
      <div className="page-head">
        <div>
          <p className="eyebrow">CURSOR · LOCAL CONNECTION</p>
          <h1>电脑上的 Cursor，额度可见</h1>
          <p className="lead">本机只读 Cursor 登录态并调用 api2.cursor.sh；令牌不会上传服务器。</p>
        </div>
        <button className="btn secondary" onClick={() => void reload()}>
          刷新
        </button>
      </div>
      <div className="sync-banner">
        <span className={"status-dot" + (error ? " offline" : "")} />
        <strong>{error ? "离线缓存" : "每 5 分钟同步"}</strong>
        <span>与 Codex 共用 bridge.json · 超过 11 分钟标为过期</span>
      </div>
      {error && (
        <p role="status" className="error">
          {error}
        </p>
      )}
      <div className="codex-grid">
        {bridges.map((b) => {
          const stale = b.received_at && tick - new Date(b.received_at).getTime() > CODEX_STALE_MS;
          return (
            <article key={b.id} className="panel codex-card">
              <div className="page-head">
                <h2>{b.name}</h2>
                <span className="device-tag">
                  {b.revoked_at ? "已撤销" : !b.received_at ? "等待电脑连接" : stale ? "数据已过期" : error ? "缓存快照" : "已连接"}
                </span>
              </div>
              <p className="muted compact">
                {b.received_at ? "最近上报 " + new Date(b.received_at).toLocaleString() : "启动电脑采集程序后，额度会显示在 Cursor 看板。"}
              </p>
              <button className="btn ghost" disabled={!!b.revoked_at} onClick={() => void revoke(b)}>
                撤销此连接
              </button>
            </article>
          );
        })}
      </div>
      {!bridges.length && (
        <div className="empty-notes">
          <span className="empty-icon">↗</span>
          <h2>连接第一台电脑</h2>
          <p>
            先在电脑登录 Cursor，再创建连接并启动采集程序。
            <br />
            手机只需登录相同的 AI Hub 账户。
          </p>
        </div>
      )}
      <div className="panel" style={{ marginTop: 24 }}>
        <h2>添加电脑连接</h2>
        <p className="muted">与 Codex 共用同一 bridge 令牌；配置中启用 cursor_enabled 即可同时采集。</p>
        <label className="field">
          <span>电脑名称</span>
          <input value={name} maxLength={60} onChange={(e) => setName(e.target.value)} />
        </label>
        <button className="btn primary" disabled={busy || !name.trim()} onClick={() => void connect()}>
          生成连接配置
        </button>
        {token && (
          <div className="sync-banner">
            <span>配置仅在本次创建后提供，请下载并妥善保管。</span>
            <button className="btn secondary" onClick={download}>
              下载 bridge.json
            </button>
            <button className="btn ghost" onClick={() => setToken("")}>
              隐藏配置
            </button>
          </div>
        )}
      </div>
    </section>
  );
}
