const fs = require('node:fs/promises');
const path = require('node:path');
const {createHash} = require('node:crypto');

// This external guard still runs if the application bundle cannot start.
const startupGuard = `setTimeout(() => {
  const root = document.getElementById('root');
  if (!root || root.childElementCount) return;
  const panel = document.createElement('section');
  panel.setAttribute('role', 'alert');
  panel.style.cssText = 'max-width:480px;margin:15vh auto;padding:32px;font:16px/1.8 system-ui;color:CanvasText;background:Canvas;border:1px solid GrayText;border-radius:16px';
  const title = document.createElement('h1'); title.textContent = '界面未能启动';
  const message = document.createElement('p'); message.textContent = '请重新加载。如果仍无法显示，请安装最新版本。已有登录和本地数据会保留。';
  const retry = document.createElement('button'); retry.textContent = '重新加载';
  retry.style.cssText = 'padding:10px 20px;font:inherit;cursor:pointer';
  retry.addEventListener('click', () => location.reload());
  panel.append(title, message, retry); root.append(panel);
}, 10000);`;

function scriptPolicy(html) {
  // precache.mjs inlines the production JS. Authorize only those exact bytes,
  // after the newline normalization performed by the HTML parser.
  const hashes = [...html.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script\s*>/gi)]
    .filter(match => match[1].trim())
    .map(match => "'sha256-" + createHash('sha256').update(match[1].replace(/\r\n?/g,'\n')).digest('base64') + "'");
  return ["'self'", ...new Set(hashes)].join(' ');
}

const routes = new Set(['/', '/login', '/notes', '/connections', '/cursor/connections', '/cursor', '/codex', '/dashboard', '/accounts/new', '/rules', '/inbox', '/devices', '/conversations', '/config', '/workspaces', '/promotions', '/admin', '/choose', '/desktop', '/timezone']);
const types = {'.html':'text/html; charset=utf-8','.js':'text/javascript; charset=utf-8','.css':'text/css; charset=utf-8','.svg':'image/svg+xml','.png':'image/png','.ico':'image/x-icon','.webmanifest':'application/manifest+json','.json':'application/json; charset=utf-8'};

// Serve only bundled application files. API traffic retains the existing origin,
// authentication, TLS and browser storage partition across installation updates.
function createHandler({root, getOrigin, forward}) {
  return async request => {
    const url = new URL(request.url);
    if (url.origin !== getOrigin() || url.pathname.startsWith('/api/') || ['/health','/ready'].includes(url.pathname)) return forward(request);
    if (!['GET','HEAD'].includes(request.method)) return new Response('Method not allowed', {status:405});
    if (url.pathname === '/__desktop_startup.js') return new Response(request.method === 'HEAD' ? null : startupGuard, {headers:{'Content-Type':types['.js'],'Cache-Control':'no-store','X-Content-Type-Options':'nosniff'}});
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
      let data = await fs.readFile(file);
      let scripts = "'self'";
      if (path.extname(file) === '.html') {
        const html = data.toString('utf8').replace(/<\/head>/i, '<script defer src="/__desktop_startup.js"></script></head>');
        scripts = scriptPolicy(html);
        data = Buffer.from(html);
      }
      return new Response(request.method==='HEAD' ? null : data,{headers:{'Content-Type':types[path.extname(file)]||'application/octet-stream','Cache-Control':'no-store','X-Content-Type-Options':'nosniff','Content-Security-Policy':`default-src 'self'; script-src ${scripts}; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'self' https: http://127.0.0.1:* http://localhost:* wss: ws://127.0.0.1:*; object-src 'none'; base-uri 'none'; frame-src 'none'; form-action 'self'`}});
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
