export type Profile = { id: string; name: string; url: string };
export type Session = { blocked?: boolean; token: string; refresh_token: string; device_id: string; user: { id: string; email: string; role?: string } };
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
 if(old && old.url !== p.url) saveSession(null,old);
 const list = profiles().filter(x => x.id !== id); list.push(p);
 localStorage.setItem("hub_profiles", JSON.stringify(list)); return p;
}
export function selectProfile(id: string) { localStorage.setItem("hub_profile", id); location.assign("/login"); }
export function removeProfile(id: string) {
 const removed = profiles().find(x => x.id === id);
 const selected = profile().id === id;
 const rest = profiles().filter(x => x.id !== id); if (!rest.length) throw new Error("请至少保留一个服务器");
 if (removed) saveSession(null, removed);
 localStorage.setItem("hub_profiles", JSON.stringify(rest));
 if (selected) localStorage.setItem("hub_profile", rest[0].id);
}
export function getSession(p = profile()): Session | null { try { if(window.aihubDesktop?.sessionGet){localStorage.removeItem("hub_session:"+p.id);return window.aihubDesktop.sessionGet(p.id);}const key="hub_session:"+p.id;if(!window.aihubDesktop)localStorage.removeItem(key);return JSON.parse(sessionStorage.getItem(key)||(window.aihubDesktop?localStorage.getItem(key):null)||"null"); } catch { return null; } }
export function saveSession(s: Session | null, p = profile()) {
 const key="hub_session:"+p.id;
 if(window.aihubDesktop?.sessionSet){localStorage.removeItem(key);if(!window.aihubDesktop.sessionSet(p.id,s))throw new Error("系统凭据保护不可用");return;}
 if (s) sessionStorage.setItem(key, JSON.stringify(s)); else sessionStorage.removeItem(key);
 if(window.aihubDesktop){ if(s)localStorage.setItem(key,JSON.stringify(s)); else localStorage.removeItem(key); }
 else localStorage.removeItem(key);
}
export function scope() { const s = getSession(); if (!s) throw new Error("请先登录"); return profile().url + "|" + s.user.id + "|" + s.device_id; }
export function clearScopedCaches() {
 const key=scope();
 for(let i=localStorage.length-1;i>=0;i--){const item=localStorage.key(i);if(item&&(item.endsWith(key)||item.includes(key+':')))localStorage.removeItem(item);}
}
if(typeof window!=="undefined"&&window.aihubDesktop&&!window.aihubDesktop.sessionGet){
 try{
  const id=localStorage.getItem("hub_profile")||"default";
  const key="hub_session:"+id;
  const existing=sessionStorage.getItem(key)||localStorage.getItem(key);
  if(existing){
   localStorage.setItem(key,existing);
   localStorage.setItem("hub_profile",id);
   if(!localStorage.getItem("hub_profiles"))localStorage.setItem("hub_profiles",JSON.stringify([{id,name:"公网 AI Hub",url:location.origin}]));
  }
 }catch{/* the older desktop shell reads this profile to start its one collector */}
}

