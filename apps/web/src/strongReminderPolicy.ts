export type Alert = {
  id: string; notified_at: string | null; read_at?: string | null;
  dismissed: boolean; snoozed_until: string | null;
  advice: { kind: string; title: string; body: string; priority: number; interrupt?: boolean; reason?: string; source_name?: string };
};

export function alertState(a: Alert, now: number) {
  if (a.dismissed) return 'dismissed';
  if (a.read_at) return 'read';
  if (a.snoozed_until && Date.parse(a.snoozed_until) > now) return 'snoozed';
  return 'pending';
}

export function eligibleStrong(a: Alert, now: number) {
  return alertState(a, now) === 'pending' &&
    !!a.notified_at && Number.isFinite(Date.parse(a.notified_at)) &&
    a.advice.interrupt !== false && ['slow', 'credit'].includes(a.advice.kind);
}

export type GenericReminder = {
  id: string; title: string; body: string; severity: string; status: string;
  created_at: string; active?: boolean; dedupe_key?: string; provider_name?: string; provider_slug?: string;
};

export function eligibleGeneric(n: GenericReminder) {
  // Codex quota is evaluated by the advisor. Legacy rules must not bypass it.
  return n.active !== false && n.status === 'unread' && ['warning', 'critical'].includes(n.severity) &&
    !(n.provider_slug === 'codex' && n.dedupe_key?.startsWith('quota.')) &&
    !n.dedupe_key?.startsWith('quota.reset_soon_unused:');
}

export type Candidate = {
  source: 'codex' | 'generic'; id: string; token: string; priority: number;
  title: string; body: string; provider?: string;
};

export function reminderCandidates(alerts: Alert[], notifications: GenericReminder[], now: number): Candidate[] {
  return [
    ...alerts.filter(a => eligibleStrong(a, now)).map(a => ({
      source: 'codex' as const, id: a.id, token: a.notified_at!,
      ...a.advice, title: a.advice.source_name ? `${a.advice.source_name} · ${a.advice.title}` : a.advice.title, provider: 'Codex',
    })),
    ...notifications.filter(eligibleGeneric).map(n => ({
      source: 'generic' as const, id: n.id, token: n.id,
      priority: n.severity === 'critical' ? 4 : 2,
      title: n.provider_name ? `${n.provider_name} · ${n.title}` : n.title,
      body: n.body, provider: n.provider_name,
    })),
  ].sort((a, b) => b.priority - a.priority || a.id.localeCompare(b.id));
}

export type DeliveryStamp = { at: number; priority: number };
export function canInterrupt(last: DeliveryStamp | null, priority: number, now: number) {
  if (!last || !Number.isFinite(last.at) || !Number.isFinite(last.priority) || last.at > now + 120000) return true;
  return priority > last.priority || now - last.at >= 10 * 60 * 1000;
}
