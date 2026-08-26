import { FormEvent, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ApiError, api, setToken } from "../api";

export default function Login() {
  const nav = useNavigate();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("demo@aihub.local");
  const [password, setPassword] = useState("demo1234");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const res = mode === "login"
        ? await api.login(email, password)
        : await api.register(email, password);
      setToken(res.token);
      nav("/");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "无法连接服务");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="auth-shell">
      <form className="auth-card" onSubmit={onSubmit}>
        <p className="eyebrow">Self-hosted AI asset hub</p>
        <h1>先看清额度，再决定要不要续。</h1>
        <p className="lead">把各平台套餐、剩余额度、重置和过期时间放在同一块看板上，并用规则生成可信提醒。</p>
        {error ? <p className="error">{error}</p> : null}
        <label className="field">
          <span>邮箱</span>
          <input value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" />
        </label>
        <label className="field">
          <span>密码</span>
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
        </label>
        <div className="actions">
          <button className="btn" disabled={busy}>{mode === "login" ? "进入看板" : "创建账户"}</button>
          <button type="button" className="btn secondary" onClick={() => setMode(mode === "login" ? "register" : "login")}>
            {mode === "login" ? "没有账户？注册" : "已有账户？登录"}
          </button>
        </div>
      </form>
    </div>
  );
}
