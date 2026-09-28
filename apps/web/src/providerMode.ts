import { scope } from "./session";

export type ActiveProvider = "codex" | "cursor";

const KEY = () => `hub_active_provider:${scope()}`;

export function getActiveProvider(): ActiveProvider {
  const raw = localStorage.getItem(KEY());
  if (raw === "cursor") return "cursor";
  return "codex";
}

export function setActiveProvider(mode: ActiveProvider) {
  localStorage.setItem(KEY(), mode);
  window.dispatchEvent(new CustomEvent("aihub-provider-mode", { detail: mode }));
}

export function providerLabel(mode: ActiveProvider) {
  return mode === "cursor" ? "Cursor 模式" : "Codex 模式";
}

type AccountHint = { provider: { slug: string }; external_account_hint?: string };

export function defaultProviderFromAccounts(accounts: AccountHint[]): ActiveProvider {
  const saved = localStorage.getItem(KEY());
  if (saved === "cursor" || saved === "codex") return saved;
  const hasCursorAuto = accounts.some((a) => a.provider.slug === "cursor" && a.external_account_hint?.startsWith("bridge:"));
  const hasCursorManual = accounts.some((a) => a.provider.slug === "cursor");
  if (hasCursorAuto || (hasCursorManual && !accounts.some((a) => a.provider.slug === "codex"))) return "cursor";
  return "codex";
}

export async function bootstrapProviderMode(loadAccounts: () => Promise<AccountHint[]>) {
  const saved = localStorage.getItem(KEY());
  if (saved === "cursor" || saved === "codex") return saved as ActiveProvider;
  try {
    const accounts = await loadAccounts();
    const mode = defaultProviderFromAccounts(accounts);
    localStorage.setItem(KEY(), mode);
    window.dispatchEvent(new CustomEvent("aihub-provider-mode", { detail: mode }));
    return mode;
  } catch {
    return "codex" as ActiveProvider;
  }
}
