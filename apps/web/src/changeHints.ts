import { getToken } from "./api";
import { profile } from "./session";

export type ChangeHint = { seq: number; type: string; at?: string; data?: Record<string, string> };

// INH-513: live change hints. WebSocket is the primary delivery on the same
// route; the SSE channel stays as the fallback for proxies that break WS.
// Both carry identical semantics: hello on connect, Last-Event-ID replay after
// a reconnect, coarse pointers only.
export function subscribeChangeHints(onEvent: (ev: ChangeHint) => void): () => void {
  let closed = false;
  let ws: WebSocket | null = null;
  let source: EventSource | null = null;
  let retry: ReturnType<typeof setTimeout> | null = null;

  const stop = () => { closed = true; cleanup(); };
  function cleanup() {
    if (retry) { clearTimeout(retry); retry = null; }
    if (ws) { ws.onclose = ws.onerror = ws.onmessage = null; try { ws.close(); } catch {} ws = null; }
    if (source) { source.onerror = source.onmessage = null; source.close(); source = null; }
  }

  function open() {
    if (closed) return;
    const base = profile().url;
    const token = getToken();
    if (!token) return;
    let lastSeq = 0;
    ws = new WebSocket(`${base}/api/v1/change-hints?access_token=${encodeURIComponent(token)}`);
    ws.onmessage = (m) => {
      try {
        const ev = JSON.parse(m.data) as ChangeHint;
        if (typeof ev.seq === "number") lastSeq = ev.seq;
        onEvent(ev);
      } catch { /* ignore malformed frames */ }
    };
    ws.onclose = () => {
      if (closed) return;
      ws = null;
      // WS unavailable (proxy, old server): drop to SSE, then back to WS after
      // a pause so a fixed proxy is picked up again.
      fallback(lastSeq);
    };
    ws.onerror = () => { /* handled by onclose */ };
  }

  function fallback(lastSeq: number) {
    if (closed) return;
    try {
      const base = profile().url;
      const token = getToken();
      if (!token) return;
      source = new EventSource(`${base}/api/v1/change-hints?access_token=${encodeURIComponent(token)}&last_event_id=${lastSeq}`);
      source.onmessage = (m) => { try { onEvent(JSON.parse(m.data) as ChangeHint); } catch {} };
      source.onerror = () => {
        if (closed) return;
        cleanup();
        retry = setTimeout(open, 30000);
      };
      // Prefer WS again on the next natural reconnect window.
      retry = setTimeout(() => { if (!closed) { cleanup(); open(); } }, 300000);
    } catch {
      retry = setTimeout(open, 30000);
    }
  }

  open();
  return stop;
}
