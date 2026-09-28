import { FormEvent, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ApiError, Provider, api } from "../api";
import { getActiveProvider } from "../providerMode";

export default function AccountNew() {
  const nav = useNavigate();
  const [providers, setProviders] = useState<Provider[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [form, setForm] = useState({
    provider_id: "",
    display_name: "",
    plan_name: "",
    region: "",
    quota_type: "credit",
    unit: "credit",
    limit_value: "",
    remaining_value: "",
    reset_policy: "fixed_time",
    reset_at: "",
    expires_at: "",
    renews_at: "",
    note: "",
  });

  useEffect(() => {
    const mode = getActiveProvider();
    api.providers().then((r) => {
      setProviders(r.providers);
      const preferred = r.providers.find((p) => p.slug === mode) ?? r.providers[0];
      if (preferred) setForm((f) => ({ ...f, provider_id: preferred.id }));
    }).catch((e) => setError(e.message));
  }, []);

  function set<K extends keyof typeof form>(k: K, v: string) {
    setForm((f) => ({ ...f, [k]: v }));
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const created = await api.createAccount({
        provider_id: form.provider_id,
        display_name: form.display_name,
        region: form.region,
        plan_name: form.plan_name,
        renews_at: toRFC(form.renews_at),
        expires_at: toRFC(form.expires_at),
        quota: {
          quota_type: form.quota_type,
          unit: form.unit,
          limit_value: num(form.limit_value),
          remaining_value: num(form.remaining_value),
          reset_policy: form.reset_policy,
          reset_at: toRFC(form.reset_at),
          expires_at: toRFC(form.expires_at),
          note: form.note,
        },
      });
      nav(`/accounts/${created.id}`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "保存失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <div className="top">
        <div>
          <p className="eyebrow">Manual first</p>
          <h1>手工登记额度</h1>
          <p>手工录入是所有平台的最终 fallback。数据来源会标记为用户手工。</p>
        </div>
      </div>
      <form className="panel" onSubmit={onSubmit}>
        {error ? <p className="error">{error}</p> : null}
        <div className="row">
          <label className="field">
            <span>平台</span>
            <select value={form.provider_id} onChange={(e) => set("provider_id", e.target.value)}>
              {providers.map((p) => <option key={p.id} value={p.id}>{p.display_name}</option>)}
            </select>
          </label>
          <label className="field">
            <span>账户名称</span>
            <input value={form.display_name} onChange={(e) => set("display_name", e.target.value)} required placeholder="例如：工作号" />
          </label>
        </div>
        <div className="row">
          <label className="field">
            <span>套餐</span>
            <input value={form.plan_name} onChange={(e) => set("plan_name", e.target.value)} placeholder="Pro / Plus / Team" />
          </label>
          <label className="field">
            <span>地区</span>
            <input value={form.region} onChange={(e) => set("region", e.target.value)} placeholder="US / CN / JP" />
          </label>
        </div>
        <div className="row">
          <label className="field">
            <span>额度类型</span>
            <select value={form.quota_type} onChange={(e) => set("quota_type", e.target.value)}>
              <option value="credit">credit</option>
              <option value="token">token</option>
              <option value="request">request</option>
              <option value="message">message</option>
              <option value="currency">currency</option>
              <option value="unknown">unknown</option>
            </select>
          </label>
          <label className="field">
            <span>单位</span>
            <input value={form.unit} onChange={(e) => set("unit", e.target.value)} />
          </label>
        </div>
        <div className="row">
          <label className="field">
            <span>总额度</span>
            <input value={form.limit_value} onChange={(e) => set("limit_value", e.target.value)} inputMode="decimal" />
          </label>
          <label className="field">
            <span>剩余额度</span>
            <input value={form.remaining_value} onChange={(e) => set("remaining_value", e.target.value)} inputMode="decimal" />
          </label>
        </div>
        <div className="row">
          <label className="field">
            <span>重置时间</span>
            <input type="datetime-local" value={form.reset_at} onChange={(e) => set("reset_at", e.target.value)} />
          </label>
          <label className="field">
            <span>过期时间</span>
            <input type="datetime-local" value={form.expires_at} onChange={(e) => set("expires_at", e.target.value)} />
          </label>
        </div>
        <div className="row">
          <label className="field">
            <span>续费时间</span>
            <input type="datetime-local" value={form.renews_at} onChange={(e) => set("renews_at", e.target.value)} />
          </label>
          <label className="field">
            <span>重置策略</span>
            <select value={form.reset_policy} onChange={(e) => set("reset_policy", e.target.value)}>
              <option value="fixed_time">固定时间</option>
              <option value="rolling_window">滚动窗口</option>
              <option value="subscription_cycle">订阅周期</option>
              <option value="manual">手工</option>
              <option value="unknown">未知</option>
            </select>
          </label>
        </div>
        <label className="field">
          <span>刷新备注</span>
          <input value={form.note} onChange={(e) => set("note", e.target.value)} placeholder="例如：从官网用量页抄录" />
        </label>
        <button className="btn" disabled={busy}>保存并求值提醒</button>
      </form>
    </>
  );
}

export function toRFC(v: string) {
  if (!v) return "";
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return "";
  return d.toISOString();
}

function num(v: string) {
  if (!v.trim()) return null;
  const n = Number(v);
  return Number.isFinite(n) ? n : null;
}
