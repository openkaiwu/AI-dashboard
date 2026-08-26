import { useEffect, useState } from "react";
import { Preview, Rule, api } from "../api";

const TYPE_LABEL: Record<string, string> = {
  "quota.low": "低额度",
  "quota.reset_soon_unused": "即将重置但剩余过多",
  "quota.expire_soon_unused": "即将过期但仍有剩余",
  "entitlement.renewal_soon": "即将续费",
  "connector.stale": "数据过久未刷新",
};

export default function Rules() {
  const [rules, setRules] = useState<Rule[]>([]);
  const [previews, setPreviews] = useState<Record<string, Preview>>({});
  const [error, setError] = useState("");

  async function load() {
    const r = await api.rules();
    setRules(r.rules);
  }

  useEffect(() => {
    load().catch((e) => setError(e.message));
  }, []);

  return (
    <>
      <div className="top">
        <div>
          <p className="eyebrow">Rule engine</p>
          <h1>提醒规则</h1>
          <p>额度变化会立即求值。时间型规则即使没有新快照也会在后台扫描。</p>
        </div>
      </div>
      {error ? <p className="error">{error}</p> : null}
      {rules.map((rule) => (
        <section className="panel" key={rule.id}>
          <div className="top" style={{ marginBottom: 8 }}>
            <div>
              <h2>{rule.name}</h2>
              <p className="meta">{TYPE_LABEL[rule.rule_type] ?? rule.rule_type} · 参数 {JSON.stringify(rule.params)}</p>
            </div>
            <label className="meta">
              <input
                type="checkbox"
                checked={rule.enabled}
                onChange={async (e) => {
                  await api.patchRule(rule.id, { enabled: e.target.checked });
                  await load();
                }}
              /> 启用
            </label>
          </div>
          <button className="btn secondary" onClick={async () => {
            const p = await api.previewRule(rule.id);
            setPreviews((m) => ({ ...m, [rule.id]: p }));
          }}>按当前数据预览</button>
          {previews[rule.id] ? (
            <div className="preview">
              <p className={previews[rule.id].would_fire ? "fire" : "ok"}>
                {previews[rule.id].would_fire ? "将触发" : "不会触发"}
              </p>
              <ul>
                {previews[rule.id].matches.map((m, i) => (
                  <li key={i}>
                    <b>{m.provider} / {m.account}</b> · {m.would_fire ? "将触发" : "不会触发"} · {m.reason}
                    {m.estimated_trigger_at ? ` · 预计 ${new Date(m.estimated_trigger_at).toLocaleString("zh-CN")}` : ""}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </section>
      ))}
    </>
  );
}
