import { useEffect, useRef, useState } from 'react';
import { request } from './api';
import { scope, accountScope } from './session';
import { canInterrupt, reminderCandidates, type Alert, type GenericReminder, type DeliveryStamp } from './strongReminderPolicy';
import { CODEX_POLL_MS } from './codexTiming';

type Popup = { title: string; body: string; provider?: string };
declare global { interface Window { aihubDesktop?: {
  alert: (value: Popup) => Promise<string>; openCodex: () => Promise<void>;
  openDashboard?: () => Promise<void>; sessionGet: (id: string) => import('./session').Session | null;
  sessionSet: (id: string, value: import('./session').Session | null) => boolean;
  installationID?: (previous: string | null) => string; extensionPairCode: () => Promise<string>;
  configureServer: (url: string) => Promise<string>;
} } }

export default function StrongReminders() {
  const [popup, setPopup] = useState<Popup | null>(null), [error, setError] = useState('');
  const dialog = useRef<HTMLDialogElement>(null), resolve = useRef<((value: string) => void) | null>(null);
  const key = 'hub_advisor:' + scope();
  const deliveryKey = 'hub_reminders:' + accountScope();
  useEffect(() => { if (popup) dialog.current?.showModal(); else dialog.current?.close(); }, [popup]);
  useEffect(() => {
    let alive = true, busy = false;
    const controller = new AbortController();
    async function display(value: Popup) {
      if (window.aihubDesktop) return window.aihubDesktop.alert(value);
      return new Promise<string>(done => { resolve.current = done; setPopup(value); });
    }
    async function poll() {
      if (busy) return;
      busy = true;
      try {
        const run = async () => {
          const strong = localStorage.getItem(key + ':strong') !== 'off';
          const system = !window.aihubDesktop && localStorage.getItem(key + ':notify') === 'on' &&
            'Notification' in window && Notification.permission === 'granted';
          if (!strong && !system) return;
          let genericUnavailable = false;
          const [overview, generic] = await Promise.all([
            request<{ generated_at: string; quiet: boolean; preferences: { enabled: boolean }; alerts: Alert[] }>('/api/v1/codex/overview', { signal: controller.signal }),
            request<{ notifications: GenericReminder[] }>('/api/v1/notifications', { signal: controller.signal }).catch(() => { genericUnavailable = true; return { notifications: [] }; }),
          ]);
          const age = Date.now() - Date.parse(overview.generated_at);
          if (!alive || overview.quiet || !Number.isFinite(age) || age < -120000 || age > 60000) return;
          const candidates = reminderCandidates(overview.preferences.enabled ? overview.alerts : [], generic.notifications, Date.now());
          const sourceWarning = genericUnavailable ? '通用通知暂不可读，Codex 提醒仍在正常检查。' : '';
          // Search past already delivered entries instead of blocking on the first row.
          const next = candidates.find(n => {
            const delivered = localStorage.getItem(`${deliveryKey}:seen:${n.source}:${n.id}`) || localStorage.getItem(`${key}:strong-seen:${n.source === 'generic' ? 'gen' : 'codex'}:${n.id}`);
            return delivered !== n.token && (n.source !== 'codex' || localStorage.getItem(`${key}:sent:${n.id}`) !== n.token);
          });
          if (!next) { setError(sourceWarning); return; }
          const seenKey = `${deliveryKey}:seen:${next.source}:${next.id}`;
          let last: DeliveryStamp | null = null;
          try { last = JSON.parse(localStorage.getItem(deliveryKey + ':last') || 'null'); } catch { /* Ignore a malformed local stamp. */ }
          if (!canInterrupt(last, next.priority, Date.now())) return;
          const deliveryPath = `/api/v1/codex/alerts/${encodeURIComponent(next.id)}/delivery`;
          let claim = '';
          if (next.source === 'codex') {
            const result = await request<{ claimed: boolean; claim_id?: string }>(deliveryPath, { method: 'POST', body: JSON.stringify({ action: 'claim', token: next.token }), signal: controller.signal });
            if (!result.claimed || !result.claim_id || !alive) return;
            claim = result.claim_id;
          }
          const receipt = (action: 'delivered' | 'release') => claim ? request(deliveryPath, { method: 'POST', body: JSON.stringify({ action, claim_id: claim }), signal: controller.signal }).catch(() => null) : Promise.resolve(null);
          if (!strong) {
            try { new Notification(next.title, { body: next.body, tag: next.id }); }
            catch (e) { await receipt('release'); throw e; }
            await receipt('delivered');
            localStorage.setItem(seenKey, next.token);
            localStorage.setItem(deliveryKey + ':last', JSON.stringify({ at: Date.now(), priority: next.priority }));
            setError(sourceWarning);
            return;
          }
          const previous = localStorage.getItem(deliveryKey + ':last');
          localStorage.setItem(deliveryKey + ':last', JSON.stringify({ at: Date.now(), priority: next.priority }));
          let showing: Promise<string>;
          const restoreCadence = () => { if (previous) localStorage.setItem(deliveryKey + ':last', previous); else localStorage.removeItem(deliveryKey + ':last'); };
          try { showing = display(next); } catch (e) { restoreCadence(); await receipt('release'); throw e; }
          // Browser dialogs have already opened; remember them even if the page closes.
          if (!window.aihubDesktop) localStorage.setItem(seenKey, next.token);
          const savedReceipt = receipt('delivered');
          let action: string;
          try { action = await showing; } catch (e) { restoreCadence(); await savedReceipt; await receipt('release'); throw e; }
          await savedReceipt;
          if (action === 'retry') {
            await receipt('release');
            restoreCadence();
            return;
          }
          if (!alive || action === 'retry') return;
          // A failed action save must not turn a displayed reminder into a new popup.
          localStorage.setItem(seenKey, next.token);
          const path = next.source === 'codex' ? `/api/v1/codex/alerts/${encodeURIComponent(next.id)}` : `/api/v1/notifications/${encodeURIComponent(next.id)}/action`;
          await request(path, { method: 'POST', body: JSON.stringify({ action }), signal: controller.signal });
          if (next.source === 'generic' && action === 'snooze') localStorage.removeItem(seenKey);
          setError(sourceWarning);
          window.dispatchEvent(new Event('aihub-reminders-changed'));
        };
        if (navigator.locks) {
          await navigator.locks.request(deliveryKey + ':lock', { ifAvailable: true }, async lock => { if (lock && alive) await run(); });
        } else {
          setError('此浏览器不支持多标签页提醒锁，请在提醒中心查看和处理事件。');
        }
      } catch {
        if (alive) setError('提醒暂未连接成功，请稍后重试。操作失败时会保留待处理状态。');
      } finally { busy = false; }
    }
    const test = () => {
      if (busy) return;
      busy = true;
      void display({ title: '强提醒测试 · 额度消耗过快', body: '这是演示弹窗，不代表真实额度异常，也不会改变提醒状态。', provider: '演示' }).finally(() => { busy = false; });
    };
    window.addEventListener('aihub-test-alert', test);
    window.addEventListener('aihub-reminder-settings', poll);
    void poll();
    const timer = setInterval(() => void poll(), CODEX_POLL_MS);
    return () => {
      alive = false; controller.abort(); clearInterval(timer);
      window.removeEventListener('aihub-test-alert', test);
      window.removeEventListener('aihub-reminder-settings', poll);
      // Closing a page does not undo a reminder that was already displayed.
      resolve.current?.('closed');
    };
  }, [key, deliveryKey]);
  function finish(action: string) { setPopup(null); resolve.current?.(action); resolve.current = null; }
  const provider = popup?.provider || 'AI Hub';
  return <>{error && <p className="sync-banner" role="status">{error}</p>}
    <dialog ref={dialog} className="strong-reminder" aria-labelledby="strong-title" aria-describedby="strong-body" onCancel={e => { e.preventDefault(); finish('read'); }}>
      <p className="eyebrow">{provider} · 重要提醒</p><h2 id="strong-title">{popup?.title}</h2><p id="strong-body">{popup?.body}</p>
      <p className="muted compact">同一事件只自动提醒一次；稍后会在 1 小时后重新检查；忽略持续到本次事件失效。处理状态会同步到其他设备。</p>
      <div className="actions"><button className="btn" autoFocus onClick={() => finish('read')}>已读</button><button className="btn secondary" onClick={() => finish('snooze')}>1 小时后提醒</button><button className="btn ghost" onClick={() => finish('dismiss')}>忽略本次</button></div>
    </dialog></>;
}
