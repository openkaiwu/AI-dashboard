import { expect, it } from 'vitest';
import { supportDiagnostics } from './desktopDiagnostics';
import type { Diagnostics } from './pages/Desktop';
it('never exports log contents, paths, addresses or future unknown fields', () => {
  const marker = 'sensitive-fixture';
  const report = {version:'1',platform:'win32',electron:'44',mode:'cloud',cloud_ready:false,
    generated_at:'2026-10-28T00:00:00Z', ui_origin:marker,data_dir:marker,scan_roots:[marker],startup_message:marker,
    bridge:{running:false,paused:true,binary_present:true,configured:true,interval_seconds:30,
      codex_observed_at:null,cursor_observed_at:null,log_tail:marker,last_error:marker,server:marker,pid:123},
    future_field:marker } as Diagnostics;
  const out = supportDiagnostics(report);
  expect(JSON.stringify(out)).not.toContain(marker);
  expect(out.authorized_directory_count).toBe(1);
  expect(out.bridge.has_error).toBe(true);
});
