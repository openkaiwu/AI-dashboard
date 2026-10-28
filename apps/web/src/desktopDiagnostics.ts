import type { Diagnostics } from './pages/Desktop';

// Export only operational facts. Raw logs, paths and server/account identifiers
// remain in the local console and must never enter a support clipboard report.
export function supportDiagnostics(value: Diagnostics) {
  const b = value.bridge;
  return {
    generated_at: value.generated_at, version: value.version, platform: value.platform,
    electron: value.electron, mode: value.mode, cloud_ready: value.cloud_ready,
    authorized_directory_count: value.scan_roots.length,
    bridge: { running: b.running, paused: b.paused, binary_present: b.binary_present,
      configured: b.configured, interval_seconds: b.interval_seconds,
      codex_observed_at: b.codex_observed_at, cursor_observed_at: b.cursor_observed_at,
      has_error: !!b.last_error },
    has_startup_message: !!value.startup_message,
  };
}
