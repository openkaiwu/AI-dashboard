import { useCallback, useEffect, useRef, useState } from "react";
import { supportDiagnostics } from '../desktopDiagnostics';

export type BridgeStatus = {
  running: boolean; paused: boolean; pid: number | null;
  binary_present: boolean; configured: boolean; server: string | null;
  interval_seconds: number | null; codex_observed_at: string | null;
  cursor_observed_at: string | null; log_tail: string; last_error: string | null;
};
export type ScanEntry = {
  path: string; platform: string; size_bytes: number;
  modified_at?: string | null; secret_keys: string[]; detected_format: string;
};
export type Diagnostics = {
  generated_at: string; version: string; platform: string; electron: string;
  mode: string; ui_origin: string; cloud_ready: boolean; data_dir: string;
  bridge: BridgeStatus; scan_roots: string[]; startup_message: string | null;
};

const MAX_ROOTS = 8;
const desktop = () => window.aihubDesktop;

function formatSize(bytes: number) {
  if (bytes < 1024) return bytes + " B";
  if (bytes < 1048576) return (bytes / 1024).toFixed(1) + " KB";
  return (bytes / 1048576).toFixed(1) + " MB";
}
function formatTime(value: string | null | undefined) {
  if (!value) return "暂无数据";
  const time = Date.parse(value);
  return Number.isFinite(time) ? new Date(time).toLocaleString() : "时间无效";
}
function errorMessage(e: unknown) { return e instanceof Error ? e.message : String(e); }

export default function Desktop() {
  const ready = !!desktop()?.bridgeStatus;
  const [status, setStatus] = useState<BridgeStatus | null>(null);
  const [roots, setRoots] = useState<string[]>([]);
  const [entries, setEntries] = useState<ScanEntry[] | null>(null);
  const [scannedAt, setScannedAt] = useState<string | null>(null);
  const [diag, setDiag] = useState<Diagnostics | null>(null);
  const [diagCopied, setDiagCopied] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState<"none" | "control" | "pick" | "remove" | "scan">("none");
  const revision = useRef(0);
  const refreshing = useRef(false);
  const busyRef = useRef(busy);
  busyRef.current = busy;

  const refresh = useCallback(async () => {
    const d = desktop();
    if (!d?.bridgeStatus || refreshing.current || busyRef.current !== 'none') return;
    refreshing.current = true;
    const version = revision.current;
    try {
      const [s, r] = await Promise.all([d.bridgeStatus(), d.scanRoots?.() ?? Promise.resolve({ roots: [] })]);
      if (version !== revision.current) return;
      setStatus(s); setRoots(r.roots);
    } catch (e) { if (version === revision.current) setError(errorMessage(e)); }
    finally { refreshing.current = false; }
  }, []);
  useEffect(() => {
    if (!ready) return;
    void refresh();
    const timer = setInterval(() => { if (!document.hidden) void refresh(); }, 30000);
    return () => { clearInterval(timer); revision.current++; };
  }, [ready, refresh]);
  useEffect(() => {
    if (!ready) return;
    desktop()?.diagnostics?.().then(setDiag).catch(() => setDiag(null));
  }, [ready]);

  async function control(action: "pause" | "resume") {
    revision.current++;
    setBusy("control");
    try { setStatus(await desktop()!.bridgeControl!(action)); setError(""); }
    catch (e) { setError(errorMessage(e)); }
    finally { setBusy("none"); }
  }
  async function pick() {
    revision.current++;
    setBusy("pick");
    try { setRoots((await desktop()!.scanPick!()).roots); setEntries(null); setError(""); }
    catch (e) { setError(errorMessage(e)); }
    finally { setBusy("none"); }
  }
  async function remove(root: string) {
    revision.current++;
    setBusy("remove");
    try { setRoots((await desktop()!.scanRemove!(root)).roots); setEntries(null); setError(""); }
    catch (e) { setError(errorMessage(e)); }
    finally { setBusy("none"); }
  }
  async function runScan() {
    setBusy("scan");
    try {
      const result = await desktop()!.scanRun!();
      setEntries(result.entries); setScannedAt(new Date().toISOString()); setError("");
    } catch (e) { setError(errorMessage(e)); }
    finally { setBusy("none"); }
  }
  async function copyDiagnostics() {
    if (!diag) return;
    const text = JSON.stringify(supportDiagnostics(diag), null, 2);
    try { await navigator.clipboard.writeText(text); setDiagCopied(true); }
    catch {
      const area = document.createElement("textarea");
      area.value = text; document.body.appendChild(area); area.select();
      try { if (!document.execCommand("copy")) throw Error('copy failed'); setDiagCopied(true); }
      catch { setError('未能复制诊断，请检查剪贴板权限后重试。'); }
      finally { area.remove(); }
    }
    setTimeout(() => setDiagCopied(false), 2000);
  }

  if (!ready) return <section className="panel"><h2>桌面控制台</h2><p className="muted">此页面需要在 AI Hub 桌面应用中打开。浏览器访问只能查看云端数据。</p></section>;

  const state = !status ? "读取中…" : !status.binary_present ? "缺少采集程序，请重新安装"
    : !status.configured ? "未配置连接"
    : status.paused ? "已暂停" : status.running ? "运行中" : "未运行";
  return <section>
    <div className="page-head"><div><p className="eyebrow">DESKTOP · LOCAL CONSOLE</p><h1>桌面控制台</h1><p className="lead">控制本机采集器、授权配置扫描目录并查看诊断。扫描结果只保存在本机界面，不自动上传。</p></div><button className="btn secondary" onClick={() => void refresh()}>刷新</button></div>
    {error && <p role="status" className="error">{error}</p>}

    <div className="panel">
      <div className="page-head"><h2>采集器状态</h2><span className="device-tag">{state}</span></div>
      {!status ? <p className="muted">正在读取采集器状态…</p> : <>
        <p className="muted compact">服务器 {status.server ? <span className="mono">{status.server}</span> : "未配置"} · 采集间隔 {status.interval_seconds ? status.interval_seconds + " 秒" : "未知"}</p>
        <p className="muted compact">Codex 最近采集：{formatTime(status.codex_observed_at)} · Cursor 最近采集：{formatTime(status.cursor_observed_at)}</p>
        {status.last_error && <p className="error">最近错误：{status.last_error}</p>}
        {status.log_tail && <p className="muted compact">最近日志：{status.log_tail}</p>}
        <div style={{ display: "flex", gap: 8, marginTop: 12 }}>
          {status.paused
            ? <button className="btn primary" disabled={busy !== "none"} onClick={() => void control("resume")}>{busy === "control" ? "正在恢复…" : "恢复采集"}</button>
            : <button className="btn secondary" disabled={busy !== "none" || !status.running} onClick={() => void control("pause")}>{busy === "control" ? "正在暂停…" : "暂停采集"}</button>}
        </div>
      </>}
    </div>

    <div className="panel">
      <div className="page-head"><h2>授权目录扫描</h2>{roots.length > 0 && <span className="device-tag">{roots.length}/{MAX_ROOTS} 个目录</span>}</div>
      <p className="muted compact">扫描只识别已知配置文件（mcp.json、.codex/config.toml 等），结果只包含路径、大小、修改时间和密钥名称。文件在本机解析，仅返回密钥名称，不上传值。</p>
      {roots.length === 0
        ? <p className="muted">还没有授权目录。添加后才能在本机扫描配置文件。</p>
        : <ul className="device-list" style={{ margin: "12px 0" }}>{roots.map(root => <li className="device-card" key={root}>
            <div className="device-icon">▣</div>
            <div><span className="mono" style={{ fontSize: 13 }}>{root}</span>
              <button className="btn ghost" disabled={busy !== "none"} onClick={() => void remove(root)} aria-label={"移除授权目录 " + root}>移除</button></div>
          </li>)}</ul>}
      <div style={{ display: "flex", gap: 8, marginTop: 12 }}>
        <button className="btn secondary" disabled={busy !== "none" || roots.length >= MAX_ROOTS} onClick={() => void pick()}>{busy === "pick" ? "正在选择…" : roots.length ? "添加目录" : "选择授权目录"}</button>
        <button className="btn primary" disabled={busy !== "none" || !roots.length} onClick={() => void runScan()}>{busy === "scan" ? "正在扫描…" : "立即扫描"}</button>
      </div>
      {entries !== null && <>
        <p className="muted compact" style={{ marginTop: 12 }}>扫描于 {formatTime(scannedAt)}：发现 {entries.length} 个配置文件。</p>
        {entries.length === 0 ? <p className="muted">授权目录里没有识别到已知配置文件。可以检查目录是否包含 mcp.json 或 .codex/config.toml。</p> : <>
          <p className="muted compact">密钥名称会被列出（仅名称，不含值），供你在导入时核对所需密钥。可移植资产只保存 secret_ref 名称引用。</p>
          <table className="table">
            <thead><tr><th>文件</th><th>平台</th><th>大小</th><th>修改时间</th><th>密钥名称</th></tr></thead>
            <tbody>{entries.map(entry => <tr key={entry.path}>
              <td><span className="mono" style={{ fontSize: 12 }}>{entry.path}</span></td>
              <td>{entry.platform}</td>
              <td>{formatSize(entry.size_bytes)}</td>
              <td>{formatTime(entry.modified_at)}</td>
              <td>{entry.secret_keys.length ? entry.secret_keys.join("、") : "无"}</td>
            </tr>)}</tbody>
          </table>
        </>}
      </>}
    </div>

    <div className="panel">
      <div className="page-head"><h2>诊断</h2><button className="btn ghost" disabled={!diag} onClick={() => void copyDiagnostics()}>{diagCopied ? "已复制" : "复制脱敏诊断"}</button></div>
      {!diag ? <p className="muted">正在生成本机诊断…</p> : <>
        <p className="muted compact">应用版本 {diag.version} · Electron {diag.electron} · {diag.platform === "win32" ? "Windows" : diag.platform === "darwin" ? "macOS" : diag.platform} · 模式 {diag.mode === "local" ? "离线本地" : "登录服务器"}</p>
        <p className="muted compact">界面地址 <span className="mono">{diag.ui_origin}</span> · 服务器就绪 {diag.cloud_ready ? "是" : "否"}</p>
        <p className="muted compact">数据目录 <span className="mono" style={{ fontSize: 12 }}>{diag.data_dir}</span></p>
        {diag.startup_message && <p className="error">启动信息：{diag.startup_message}</p>}
      </>}
    </div>
  </section>;
}
