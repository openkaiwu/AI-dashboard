import StrongReminders from "./StrongReminders";
import { NavLink, Navigate, Outlet, Route, Routes, useNavigate } from "react-router-dom";
import { useEffect, useState } from "react";
import { api, getToken } from "./api";
import { getSession, profile, saveSession } from "./session";
import Login from "./pages/Login";
import Dashboard from "./pages/Dashboard";
import AccountNew from "./pages/AccountNew";
import AccountDetail from "./pages/AccountDetail";
import Rules from "./pages/Rules";
import Inbox from "./pages/Inbox";
import Sync from "./pages/Sync";
import Devices from "./pages/Devices";
import Codex from "./pages/CodexOverview";
import CodexConnections from "./pages/CodexConnections";
import CursorConnections from "./pages/CursorConnections";
import { ActiveProvider, bootstrapProviderMode, getActiveProvider, providerLabel, setActiveProvider } from "./providerMode";

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route element={<RequireAuth />}>
        <Route element={<Shell />}>
          <Route path="/" element={<HomeRedirect />} />
          <Route path="/notes" element={<Sync />} />
          <Route path="/connections" element={<CodexConnections />} />
          <Route path="/cursor/connections" element={<CursorConnections />} />
          <Route path="/codex" element={<Codex />} />
          <Route path="/dashboard" element={<Dashboard />} />
          <Route path="/accounts/new" element={<AccountNew />} />
          <Route path="/accounts/:id" element={<AccountDetail />} />
          <Route path="/rules" element={<Rules />} />
          <Route path="/inbox" element={<Inbox />} />
          <Route path="/devices" element={<Devices />} />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

function RequireAuth() {
  return getToken() ? <Outlet /> : <Navigate to="/login" replace />;
}

function HomeRedirect() {
  const mode = getActiveProvider();
  return <Navigate to={mode === "cursor" ? "/dashboard" : "/codex"} replace />;
}

function Shell() {
  const nav = useNavigate();
  const [mode, setMode] = useState<ActiveProvider>(() => getActiveProvider());

  useEffect(() => {
    const onMode = (e: Event) => setMode((e as CustomEvent<ActiveProvider>).detail);
    window.addEventListener("aihub-provider-mode", onMode);
    void bootstrapProviderMode(async () => {
      const dash = await api.dashboard();
      return dash.accounts;
    }).then((m) => setMode(m));
    return () => window.removeEventListener("aihub-provider-mode", onMode);
  }, []);

  async function logout() {
    try {
      await api.logout();
      saveSession(null);
      nav("/login");
    } catch {
      alert("当前无法撤销服务器会话。请恢复网络后退出，或从其他设备撤销本设备。");
    }
  }

  function switchMode(next: ActiveProvider) {
    setActiveProvider(next);
    setMode(next);
    nav(next === "cursor" ? "/dashboard" : "/codex");
  }

  return (
    <div className="layout">
      <aside className="side">
        <div className="brand">
          <div className="brand-mark">↗</div>
          <b>AI Hub</b>
          <span>{mode === "cursor" ? "Cursor 额度助手" : "Codex 使用助手"}</span>
        </div>
        <div className="panel" style={{ margin: "0 0 16px", padding: 12 }}>
          <p className="nav-label" style={{ marginTop: 0 }}>
            工作模式
          </p>
          <div className="actions" style={{ gap: 8 }}>
            <button className={"btn" + (mode === "codex" ? "" : " ghost")} onClick={() => switchMode("codex")}>
              Codex 模式
            </button>
            <button className={"btn" + (mode === "cursor" ? "" : " ghost")} onClick={() => switchMode("cursor")}>
              Cursor 模式
            </button>
          </div>
          <p className="muted compact">当前：{providerLabel(mode)} · 采集与提醒双端继续</p>
        </div>
        {mode === "codex" ? (
          <>
            <p className="nav-label">CODEX 工作空间</p>
            <nav className="nav">
              <NavLink to="/codex">
                <span>◈</span> Codex 工作台
              </NavLink>
              <NavLink to="/notes">
                <span>▤</span> 跨端便笺
              </NavLink>
            </nav>
            <p className="nav-label">连接</p>
            <nav className="nav">
              <NavLink to="/connections">
                <span>↗</span> 电脑采集器
              </NavLink>
              <NavLink to="/devices">
                <span>▣</span> 设备管理
              </NavLink>
              <NavLink to="/login">
                <span>⇄</span> 切换服务器
              </NavLink>
            </nav>
          </>
        ) : (
          <>
            <p className="nav-label">CURSOR 工作空间</p>
            <nav className="nav">
              <NavLink to="/dashboard">
                <span>◈</span> 额度看板
              </NavLink>
              <NavLink to="/rules">
                <span>◷</span> 提醒规则
              </NavLink>
              <NavLink to="/inbox">
                <span>✉</span> 通知中心
              </NavLink>
              <NavLink to="/notes">
                <span>▤</span> 跨端便笺
              </NavLink>
            </nav>
            <p className="nav-label">连接</p>
            <nav className="nav">
              <NavLink to="/cursor/connections">
                <span>↗</span> 电脑采集器
              </NavLink>
              <NavLink to="/devices">
                <span>▣</span> 设备管理
              </NavLink>
              <NavLink to="/login">
                <span>⇄</span> 切换服务器
              </NavLink>
            </nav>
          </>
        )}
        <div className="side-foot">
          <span className="device-tag">{mode.toUpperCase()} · COMPANION</span>
          <p>{profile().name}</p>
          <div className="muted">{getSession()?.user.email}</div>
          <button className="btn ghost" onClick={() => void logout()}>
            退出此设备
          </button>
        </div>
      </aside>
      <main className="main">
        {getSession()?.user.email === "demo@aihub.local" && (
          <div className="demo-banner">
            本地工作空间 · {mode === "cursor" ? "Cursor 数据来自电脑采集或手工登记" : "Codex 数据来自电脑采集"}，建议仅供安排任务参考。
          </div>
        )}
        <StrongReminders />
        <Outlet />
      </main>
    </div>
  );
}
