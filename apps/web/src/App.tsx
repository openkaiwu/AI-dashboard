import { NavLink, Navigate, Outlet, Route, Routes, useNavigate } from "react-router-dom";
import { useEffect, useState } from "react";
import { api, getToken, setToken } from "./api";
import Login from "./pages/Login";
import Dashboard from "./pages/Dashboard";
import AccountNew from "./pages/AccountNew";
import AccountDetail from "./pages/AccountDetail";
import Rules from "./pages/Rules";
import Inbox from "./pages/Inbox";

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route element={<RequireAuth />}>
        <Route element={<Shell />}>
          <Route path="/" element={<Dashboard />} />
          <Route path="/accounts/new" element={<AccountNew />} />
          <Route path="/accounts/:id" element={<AccountDetail />} />
          <Route path="/rules" element={<Rules />} />
          <Route path="/inbox" element={<Inbox />} />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

function RequireAuth() {
  if (!getToken()) return <Navigate to="/login" replace />;
  return <Outlet />;
}

function Shell() {
  const [email, setEmail] = useState("");
  const [unread, setUnread] = useState(0);
  const nav = useNavigate();

  useEffect(() => {
    api.me().then((me) => {
      setEmail(me.email);
      setUnread(me.unread_count);
    }).catch(() => {
      setToken(null);
      nav("/login");
    });
  }, [nav]);

  return (
    <div className="layout">
      <aside className="side">
        <div className="brand">
          <b>AI Hub</b>
          <span>额度追踪 MVP</span>
        </div>
        <nav className="nav">
          <NavLink to="/" end>看板</NavLink>
          <NavLink to="/accounts/new">登记账户</NavLink>
          <NavLink to="/rules">提醒规则</NavLink>
          <NavLink to="/inbox">
            通知
            {unread > 0 ? <b>{unread}</b> : null}
          </NavLink>
        </nav>
        <div className="side-foot">
          <div>{email}</div>
          <button className="btn ghost" onClick={() => { setToken(null); nav("/login"); }}>退出</button>
        </div>
      </aside>
      <main className="main">
        <Outlet />
      </main>
    </div>
  );
}
