import { Activity, ArrowUpRight, Bell, Blocks, ChevronDown, ChevronRight, Clock3, FolderOpen, LayoutDashboard, LogOut, Menu, MessageSquare, Monitor, NotebookPen, Plug, Server, Settings2, ShieldCheck, Ticket, Users, Wrench, X, type LucideIcon } from "lucide-react";
import ThemeSwitch from "./ThemeSwitch";
import StrongReminders from "./StrongReminders";
import { NavLink, Navigate, Outlet, Route, Routes, useNavigate, useLocation } from "react-router-dom";
import { useEffect, useRef, useState } from "react";
import { api, getToken } from "./api";
import { clearScopedCaches, getSession, profile, saveSession } from "./session";
import Login from "./pages/Login";
import Dashboard from "./pages/Dashboard";
import AccountNew from "./pages/AccountNew";
import AccountDetail from "./pages/AccountDetail";
import Rules from "./pages/Rules";
import Inbox from "./pages/Inbox";
import Sync from "./pages/Sync";
import Devices from "./pages/Devices";
import Conversations from "./pages/Conversations";
import ConfigAssets from "./pages/ConfigAssets";
import Workspaces from "./pages/Workspaces";
import Promotions from "./pages/Promotions";
import Codex from "./pages/CodexOverview";
import CodexConnections from "./pages/CodexConnections";
import CursorConnections from "./pages/CursorConnections";
import Admin from "./pages/Admin";
import ModeSelect from "./pages/ModeSelect";
import Desktop from "./pages/Desktop";
import TimeZone from './pages/TimeZone';
import CursorOverview from "./pages/CursorOverview";
import { ActiveProvider, getActiveProvider, hasActiveProvider, setActiveProvider } from "./providerMode";
import { flushManual, pendingManualCount, clearManualQueue } from "./manualQueue";
import {loadState,clearSyncState} from "./sync";
import {subscribeChangeHints} from "./changeHints";

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route element={<RequireAuth />}>
        <Route element={<Shell />}>
          <Route path="/" element={<HomeRedirect />} />
          <Route path="/notes" element={<Sync />} />
          <Route path="/connections" element={<CodexConnections />} />
          <Route path="/desktop" element={<Desktop />} />
          <Route path="/timezone" element={<TimeZone />} />
          <Route path="/cursor/connections" element={<CursorConnections />} />
          <Route path="/cursor" element={<CursorOverview />} />
          <Route path="/codex" element={<Codex />} />
          <Route path="/dashboard" element={<Dashboard />} />
          <Route path="/accounts/new" element={<AccountNew />} />
          <Route path="/accounts/:id" element={<AccountDetail />} />
          <Route path="/rules" element={<Rules />} />
          <Route path="/inbox" element={<Inbox />} />
          <Route path="/devices" element={<Devices />} />
          <Route path="/conversations" element={<Conversations />} />
          <Route path="/config" element={<ConfigAssets />} />
          <Route path="/workspaces" element={<Workspaces />} />
          <Route path="/promotions" element={<Promotions />} />
          <Route path="/admin" element={<Admin />} />
          <Route path="/choose" element={<ModeSelect />} />
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
  if (!window.aihubDesktop && getSession()?.user.role === "admin") return <Navigate to="/admin" replace />;
  if (!hasActiveProvider()) return <Navigate to="/choose" replace />;
  const mode = getActiveProvider();
  return <Navigate to={mode === "cursor" ? "/cursor" : "/codex"} replace />;
}

function Shell() {
  const nav = useNavigate();
  const location = useLocation();
  const menuRef = useRef<HTMLDialogElement>(null);
  const hasTimeZone = !!window.aihubDesktop?.timeZoneStatus && window.aihubDesktop?.info?.().platform === 'win32';
  const [online, setOnline] = useState(navigator.onLine);
  useEffect(() => {
    const update = () => setOnline(navigator.onLine);
    window.addEventListener("online", update);
    window.addEventListener("offline", update);
    return () => { window.removeEventListener("online", update); window.removeEventListener("offline", update); };
  }, []);
  useEffect(() => { menuRef.current?.close(); }, [location.pathname, location.search]);
  const [mode, setMode] = useState<ActiveProvider>(() => getActiveProvider());


  useEffect(() => {
    const onMode = (e: Event) => setMode((e as CustomEvent<ActiveProvider>).detail);
    window.addEventListener("aihub-provider-mode", onMode);
    return () => window.removeEventListener("aihub-provider-mode", onMode);
  }, []);
  useEffect(()=>{
    const sync=()=>{if(getSession() && navigator.onLine) void flushManual().catch(()=>{});};
    sync();window.addEventListener('online',sync);const timer=setInterval(sync,60000);
    return()=>{window.removeEventListener('online',sync);clearInterval(timer);};
  },[]);
  useEffect(()=>{
    if(!getToken())return;
    const stop=subscribeChangeHints(ev=>{window.dispatchEvent(new CustomEvent("aihub-change-hint",{detail:ev}));});
    return stop;
  },[]);

  async function logout() {
    try {
      const pending=await pendingManualCount();
      const notes=(await loadState()).pending.length;
      if((pending||notes) && !confirm(`有 ${pending} 条额度修改和 ${notes} 条便笺修改尚未同步。退出将清除本机待同步修改，确定退出？`))return;
      await api.logout();
      await clearManualQueue();
      await clearSyncState();
      clearScopedCaches();
      saveSession(null);
      nav("/login");
    } catch {
      alert("当前无法撤销服务器会话。请恢复网络后退出，或从其他设备撤销本设备。");
    }
  }

  function switchMode(next: ActiveProvider) {
    menuRef.current?.close();
    setActiveProvider(next);
    setMode(next);
    nav(next === "cursor" ? "/cursor" : "/codex");
  }

  if (!window.aihubDesktop && getSession()?.user.role === "admin") return <main className="main"><Outlet /></main>;

  const home = mode === "cursor" ? "/cursor" : "/codex";
  const connections = mode === "cursor" ? "/cursor/connections" : "/connections";
  type Item = { to: string; label: string; icon: LucideIcon };
  const primary: Item[] = [
    { to: home, label: "工作台", icon: LayoutDashboard },
    { to: "/notes", label: "跨端便笺", icon: NotebookPen },
    ...(mode === "cursor" ? [{ to: "/inbox", label: "通知中心", icon: Bell }, { to: "/rules", label: "提醒规则", icon: Settings2 }] : []),
  ];
  const connectionItems: Item[] = [
    { to: connections, label: "电脑采集器", icon: Plug },
    ...(window.aihubDesktop ? [{ to: "/desktop", label: "桌面控制台", icon: Wrench }] : []),
    { to: "/devices", label: "设备管理", icon: Monitor },
  ];
  const extra: Item[] = mode === "codex" ? [
    { to: "/conversations", label: "会话资产", icon: MessageSquare },
    { to: "/config", label: "可移植配置", icon: FolderOpen },
    { to: "/workspaces", label: "协作工作区", icon: Users },
    { to: "/promotions", label: "优惠情报", icon: Ticket },
  ] : [];
  if (getSession()?.user.role === "admin") extra.push({ to: "/admin", label: "账户管理", icon: ShieldCheck });
  const localTools: Item[] = hasTimeZone ? [{to:'/timezone',label:'时区切换',icon:Clock3}] : [];
  const allItems = [...primary, ...connectionItems, ...localTools, ...extra];
  const currentPage = allItems.find(item => item.to === location.pathname)?.label || "工作空间";
  const renderLinks = (items: Item[]) => items.map(({ to, label, icon: Icon }) => (
    <NavLink key={to} to={to} end onClick={() => menuRef.current?.close()}><Icon size={18} aria-hidden="true"/><span>{label}</span></NavLink>
  ));
  const modePicker = <div className="mode-picker" role="group" aria-label="工作模式">
    {(["codex", "cursor"] as const).map(value => <button key={value} type="button" aria-pressed={mode === value} onClick={() => switchMode(value)}>{value === "codex" ? "Codex" : "Cursor"}</button>)}
  </div>;
  const brand = <div className="brand"><div className="brand-mark"><Activity size={23} aria-hidden="true"/></div><div><b>AI Hub<span className="brand-dot">.</span></b><span>你的 AI 工作空间</span></div></div>;

  return (
    <div className="layout">
      <a className="skip-link" href="#workspace-content">跳到主要内容</a>
      <aside className="side" aria-label="主导航">
        {brand}
        {modePicker}
        <div className="nav-group"><p className="nav-label">工作空间</p><nav className="nav">{renderLinks(primary)}</nav></div>
        <div className="nav-group"><p className="nav-label">连接与同步</p><nav className="nav">{renderLinks(connectionItems)}</nav></div>
        {hasTimeZone && <div className="nav-group"><p className="nav-label">本机工具</p><nav className="nav">{renderLinks(localTools)}</nav></div>}
        {extra.length > 0 && <details className="nav-more" open={extra.some(item => item.to === location.pathname) || undefined}><summary><Blocks size={17} aria-hidden="true"/>更多工具<ChevronDown size={15} aria-hidden="true"/></summary><nav className="nav">{renderLinks(extra)}</nav></details>}
        <div className="side-foot">
          <div className="workspace-account"><div className="account-avatar" aria-hidden="true">{(getSession()?.user.email || "A").slice(0, 1).toUpperCase()}</div><div><strong>{profile().name}</strong><span>{getSession()?.user.email}</span></div></div>
          <div className="account-actions"><NavLink to="/login"><Server size={15} aria-hidden="true"/>服务器</NavLink><button onClick={() => void logout()}><LogOut size={15} aria-hidden="true"/>退出</button></div>
        </div>
      </aside>
      <div className="workspace-shell">
        <header className="workspace-bar">
          <div className="desktop-breadcrumb"><span>我的工作空间</span><ChevronRight size={14} aria-hidden="true"/><strong>{currentPage}</strong></div>
          <div className="mobile-brand">{brand}</div>
          <div className="workspace-bar-actions"><ThemeSwitch/><span className="network-state"><span className={"status-dot" + (online ? "" : " offline")}/>{online ? "本机网络可用" : "当前离线"}</span><NavLink className="icon-button" to={home + "?tab=settings"} aria-label="提醒设置"><Settings2 size={18}/></NavLink></div>
        </header>
        <main className="main" id="workspace-content" tabIndex={-1}>
          <StrongReminders />
          <Outlet />
        </main>
      </div>
      <nav className="mobile-nav" aria-label="底部导航">
        <NavLink to={home}><LayoutDashboard size={21} aria-hidden="true"/><span>总览</span></NavLink>
        <NavLink to="/notes"><NotebookPen size={21} aria-hidden="true"/><span>便笺</span></NavLink>
        <NavLink to="/devices"><Monitor size={21} aria-hidden="true"/><span>设备</span></NavLink>
        <button type="button" onClick={() => menuRef.current?.showModal()} aria-haspopup="dialog"><Menu size={21} aria-hidden="true"/><span>更多</span></button>
      </nav>
      <dialog ref={menuRef} className="mobile-menu" aria-labelledby="mobile-menu-title">
        <div className="section-heading"><h2 id="mobile-menu-title">工作空间</h2><form method="dialog"><button className="icon-button" aria-label="关闭菜单"><X size={20}/></button></form></div>
        {modePicker}
        <nav className="nav">{renderLinks(allItems)}<NavLink to="/choose"><ArrowUpRight size={18} aria-hidden="true"/>选择工作模式</NavLink><NavLink to="/login"><Server size={18} aria-hidden="true"/>切换服务器</NavLink></nav>
        <button className="btn secondary" onClick={() => void logout()}><LogOut size={16} aria-hidden="true"/>退出此设备</button>
      </dialog>
    </div>
  );
}
