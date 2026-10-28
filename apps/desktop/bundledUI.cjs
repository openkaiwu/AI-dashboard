const fs = require('node:fs/promises');
const path = require('node:path');

const routes = new Set(['/', '/login', '/notes', '/connections', '/cursor/connections', '/cursor', '/codex', '/dashboard', '/accounts/new', '/rules', '/inbox', '/devices', '/conversations', '/config', '/workspaces', '/promotions', '/admin', '/choose', '/desktop']);
const types = {'.html':'text/html; charset=utf-8','.js':'text/javascript; charset=utf-8','.css':'text/css; charset=utf-8','.svg':'image/svg+xml','.png':'image/png','.ico':'image/x-icon','.webmanifest':'application/manifest+json','.json':'application/json; charset=utf-8'};

// Serve only bundled application files. API traffic retains the existing origin,
// authentication, TLS and browser storage partition across installation updates.
function createHandler({root, getOrigin, forward}) {
  return async request => {
    const url = new URL(request.url);
    if (url.origin !== getOrigin() || url.pathname.startsWith('/api/') || ['/health','/ready'].includes(url.pathname)) return forward(request);
    if (!['GET','HEAD'].includes(request.method)) return new Response('Method not allowed', {status:405});
    let pathname;
    try { pathname = decodeURIComponent(url.pathname); } catch { return new Response('Invalid path', {status:400}); }
    if (pathname.includes('\\') || pathname.includes('\0') || pathname.split('/').some(p=>p==='..'||p==='.') || pathname.includes(':')) return new Response('Invalid path', {status:400});
    if (pathname === '/sw.js') return new Response('Desktop UI is bundled', {status:404});
    const isRoute = routes.has(pathname) || /^\/accounts\/[a-zA-Z0-9_-]+$/.test(pathname);
    const relative = isRoute ? 'index.html' : pathname.replace(/^\//,'');
    if (!isRoute && !/^(assets\/[\w.-]+|favicon\.ico|icon-[\w.-]+|manifest\.webmanifest|schemas\/[\w.-]+\.json)$/.test(relative)) return new Response('Not found', {status:404});
    try {
      const base = await fs.realpath(root);
      const file = await fs.realpath(path.join(base, relative));
      const rel = path.relative(base,file);
      if (rel.startsWith('..') || path.isAbsolute(rel)) return new Response('Forbidden', {status:403});
      const data = request.method==='HEAD' ? null : await fs.readFile(file);
      return new Response(data,{headers:{'Content-Type':types[path.extname(file)]||'application/octet-stream','Cache-Control':'no-store','X-Content-Type-Options':'nosniff','Content-Security-Policy':"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'self' https: http://127.0.0.1:* http://localhost:* wss: ws://127.0.0.1:*; object-src 'none'; base-uri 'none'; frame-src 'none'; form-action 'self'"}});
    } catch { return new Response('Desktop UI file unavailable. Reinstall the application.',{status:503}); }
  };
}

async function install({session,root,getOrigin}) {
  await fs.access(path.join(root,'index.html'));
  const handler=createHandler({root,getOrigin,forward:req=>session.fetch(req,{bypassCustomProtocolHandlers:true})});
  session.protocol.handle('https',handler);
  session.protocol.handle('http',handler);
}
module.exports={createHandler,install};
