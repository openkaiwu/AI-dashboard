export type Profile = { id: string; name: string; url: string };
export type Session = { blocked?: boolean; token: string; refresh_token: string; device_id: string; user: { id: string; email: string } };
const DEFAULT: Profile = { id: "default", name: "当前服务器", url: location.origin };
export function profiles(): Profile[] { try { return JSON.parse(localStorage.getItem("hub_profiles") || "null") || [DEFAULT]; } catch { return [DEFAULT]; } }
export function profile(): Profile { return profiles().find(p => p.id === localStorage.getItem("hub_profile")) || profiles()[0] || DEFAULT; }
export function validateURL(input: string): string {
 const url = new URL(input);
 if (url.username || url.password || url.search || url.hash || url.pathname !== "/") throw new Error("请输入服务器根地址，不包含路径或凭据");
 if (url.protocol !== "https:" && !(url.protocol === "http:" && ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname))) throw new Error("跨网络连接必须使用 HTTPS；HTTP 仅限本机开发");
 return url.origin;
}
export function saveProfile(name: string, url: string, id: string = crypto.randomUUID()) {
 const old = profiles().find(x => x.id === id);
 const p = { id, name: name.trim() || "自部署服务器", url: validateURL(url) };
 if(old && old.url !== p.url) localStorage.removeItem("hub_session:" + id);
 const list = profiles().filter(x => x.id !== id); list.push(p);
 localStorage.setItem("hub_profiles", JSON.stringify(list)); return p;
}
export function selectProfile(id: string) { localStorage.setItem("hub_profile", id); location.assign("/login"); }
export function removeProfile(id: string) {
 const rest = profiles().filter(x => x.id !== id); if (!rest.length) throw new Error("请至少保留一个服务器");
 localStorage.setItem("hub_profiles", JSON.stringify(rest));
 localStorage.removeItem("hub_session:" + id);
 if (profile().id === id) localStorage.setItem("hub_profile", rest[0].id);
}
export function getSession(p = profile()): Session | null { try { return JSON.parse(localStorage.getItem("hub_session:" + p.id) || "null"); } catch { return null; } }
export function saveSession(s: Session | null, p = profile()) {
 if (s) localStorage.setItem("hub_session:" + p.id, JSON.stringify(s)); else localStorage.removeItem("hub_session:" + p.id);
}
export function scope() { const s = getSession(); if (!s) throw new Error("请先登录"); return profile().url + "|" + s.user.id; }

