import { getSession, saveSession, profile, Session } from "./session";
export function getToken(){return getSession()?.token||null;}
export function setToken(token:string|null){if(!token)saveSession(null);}
export class ApiError extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

export async function request<T>(path:string,init:RequestInit={},retried=false):Promise<T>{
 const p=profile();const session=getSession(p);
 if(session?.blocked&&!path.startsWith("/api/v1/auth/"))throw new ApiError(401,"session_revoked","请重新登录；本地修改仍保留");
 const headers=new Headers(init.headers);headers.set("Accept","application/json");
 if(init.body)headers.set("Content-Type","application/json");
 if(session)headers.set("Authorization","Bearer "+session.token);
 const res=await fetch(p.url+path,{...init,headers,signal:init.signal||AbortSignal.timeout(15000),redirect:"error"});
 let data:any;try{data=await res.json();}catch{throw new ApiError(res.status,"invalid_response","服务器未返回有效数据");}
 if(res.status===401&&session&&!retried&&!path.startsWith("/api/v1/auth/")){
  await navigator.locks.request("hub-refresh:"+p.id,async()=>{
   const latest=getSession(p);if(latest?.token!==session.token)return;
   const response=await fetch(p.url+"/api/v1/auth/refresh",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({refresh_token:session.refresh_token}),signal:AbortSignal.timeout(15000),redirect:"error"});
   if(!response.ok){if(response.status===401){saveSession({...session,blocked:true},p);throw new ApiError(401,"session_revoked","请重新登录；本地修改仍保留");}throw new ApiError(response.status,"refresh_unavailable","暂时无法刷新登录，请稍后重试");}
   saveSession(await response.json(),p);
  });
  if(profile().id!==p.id)throw new Error("服务器已切换");
  return request<T>(path,init,true);
 }
 if(!res.ok)throw new ApiError(res.status,data.error??"error",data.message??"请求失败");
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
  collection_status?: string;
  status: string;
};

export type AccountSummary = Account & { external_account_hint?: string };

export type Account = {
  id: string;
  display_name: string;
  external_account_hint?: string;
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
  provider_account_id?: string;
};

export type NotificationItem = {
  id: string;
  title: string;
  body: string;
  severity: string;
  status: string;
  created_at: string;
  provider_slug?: string;
  provider_name?: string;
  provider_account_id?: string;
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
 logout:()=>request("/api/v1/auth/logout",{method:"POST"}),
  register: (email: string, password: string, device_name: string) =>
    request<Session>("/api/v1/auth/register", {
      method: "POST",
      body: JSON.stringify({ email, password, device_name }),
    }),
  login: (email: string, password: string, device_name: string) =>
    request<Session>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password, device_name }),
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
