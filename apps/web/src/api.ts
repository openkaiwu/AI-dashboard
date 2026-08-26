const TOKEN_KEY = "aihub_token";

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string | null) {
  if (token) localStorage.setItem(TOKEN_KEY, token);
  else localStorage.removeItem(TOKEN_KEY);
}

export class ApiError extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const token = getToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const res = await fetch(path, { ...init, headers });
  const text = await res.text();
  const data = text ? JSON.parse(text) : {};
  if (!res.ok) {
    throw new ApiError(res.status, data.error ?? "error", data.message ?? "请求失败");
  }
  return data as T;
}

export type Provider = {
  id: string;
  slug: string;
  display_name: string;
  category: string;
  homepage: string;
  capabilities: { capability: string; support_level: string; acquisition_mode: string }[];
};

export type Bucket = {
  id: string;
  scope_key: string;
  quota_type: string;
  unit: string;
  limit_value: number | null;
  remaining_value: number | null;
  remaining_ratio: number | null;
  used_value: number | null;
  reset_policy: string;
  reset_at: string | null;
  expires_at: string | null;
  source_type: string;
  confidence: string;
  observed_at: string | null;
  note: string;
  status: string;
};

export type Account = {
  id: string;
  display_name: string;
  external_account_hint: string;
  region: string;
  provider: { id: string; slug: string; display_name: string };
  plan_name: string;
  plan_code: string;
  renews_at: string | null;
  expires_at: string | null;
  computed_status: string;
  buckets: Bucket[];
};

export type Dashboard = {
  generated_at: string;
  counts: Record<string, number>;
  accounts: Account[];
};

export type Rule = {
  id: string;
  name: string;
  rule_type: string;
  enabled: boolean;
  params: { ratio?: number; hours?: number };
  provider_account_id: string;
};

export type NotificationItem = {
  id: string;
  title: string;
  body: string;
  severity: string;
  status: string;
  created_at: string;
};

export type Preview = {
  would_fire: boolean;
  evaluated_at: string;
  matches: {
    provider: string;
    account: string;
    would_fire: boolean;
    reason: string;
    estimated_trigger_at: string | null;
  }[];
};

export type Snapshot = {
  id: string;
  observed_at: string;
  remaining_value: number | null;
  remaining_ratio: number | null;
  note: string;
  source_type: string;
};

export const api = {
  register: (email: string, password: string) =>
    request<{ token: string; user: { id: string; email: string } }>("/api/v1/auth/register", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    }),
  login: (email: string, password: string) =>
    request<{ token: string; user: { id: string; email: string } }>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    }),
  me: () => request<{ id: string; email: string; unread_count: number }>("/api/v1/me"),
  dashboard: () => request<Dashboard>("/api/v1/dashboard"),
  providers: () => request<{ providers: Provider[] }>("/api/v1/providers"),
  createAccount: (body: unknown) =>
    request<Account>("/api/v1/provider-accounts", { method: "POST", body: JSON.stringify(body) }),
  getAccount: (id: string) => request<Account>(`/api/v1/provider-accounts/${id}`),
  deleteAccount: (id: string) =>
    request<{ ok: boolean }>(`/api/v1/provider-accounts/${id}`, { method: "DELETE" }),
  manualSnapshot: (id: string, body: unknown) =>
    request<{ ok: boolean }>(`/api/v1/quota-buckets/${id}/manual-snapshot`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  snapshots: (id: string, days = 30) =>
    request<{ snapshots: Snapshot[] }>(`/api/v1/quota-buckets/${id}/snapshots?days=${days}`),
  rules: () => request<{ rules: Rule[] }>("/api/v1/notification-rules"),
  patchRule: (id: string, body: unknown) =>
    request<{ ok: boolean }>(`/api/v1/notification-rules/${id}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  previewRule: (id: string) =>
    request<Preview>(`/api/v1/notification-rules/${id}/preview`, { method: "POST" }),
  notifications: () => request<{ notifications: NotificationItem[] }>("/api/v1/notifications"),
  readNotification: (id: string) =>
    request<{ ok: boolean }>(`/api/v1/notifications/${id}/read`, { method: "POST" }),
  readAll: () => request<{ ok: boolean }>("/api/v1/notifications/read-all", { method: "POST" }),
};
