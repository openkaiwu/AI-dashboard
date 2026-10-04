const CACHE="aihub-shell-91f3b93bcb80";
const PRECACHE=["/","/manifest.webmanifest","/icon.svg","/assets/index-CjnqDacv.js","/assets/index-L-da4xuc.css"];
self.addEventListener("install",event=>{
 event.waitUntil(caches.open(CACHE).then(cache=>cache.addAll(PRECACHE)).then(()=>self.skipWaiting()));
});
self.addEventListener("activate",event=>{
 event.waitUntil((async()=>{await self.clients.claim();for(const key of await caches.keys()){if(key.startsWith("aihub-shell-")&&key!==CACHE)await caches.delete(key);}})());
});
self.addEventListener("fetch",event=>{
 const request=event.request,url=new URL(request.url);
 if(request.method!=="GET"||url.origin!==location.origin||url.pathname.startsWith("/api/")||url.pathname==="/health"||url.pathname==="/ready")return;
 event.respondWith((async()=>{
  const cache=await caches.open(CACHE);
  // Content-hashed bundles are immutable; serve their verified precache first.
  const cached=await cache.match(request);
  if(url.pathname.startsWith("/assets/")&&cached)return cached;
  try{
   const response=await fetch(request);
   if(response.ok)await cache.put(request,response.clone());
   return response;
  }catch{
   return cached||(request.mode==="navigate"?await cache.match("/"):undefined)||new Response("Offline asset unavailable",{status:503});
  }
 })());
});
