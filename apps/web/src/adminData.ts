import { friendlyError } from './errors';

export const adminSections = [
 ['users', '账户', '/api/v1/admin/users'],
 ['report', '观察报表', '/api/v1/admin/telemetry'],
 ['ops', '运维状态', '/api/v1/admin/operations'],
 ['regs', '注册审核', '/api/v1/admin/registrations?status=all'],
 ['invites', '邀请码', '/api/v1/admin/invites'],
 ['radarInfo', '雷达令牌', '/api/v1/admin/radar-token'],
] as const;

export async function loadAdminSections(fetchSection: (path: string) => Promise<unknown>) {
 const results = await Promise.allSettled(adminSections.map(([, , path]) => fetchSection(path)));
 const data: Record<string, any> = {};
 const errors: string[] = [];
 results.forEach((result, index) => {
  const [key, label] = adminSections[index];
  if (result.status === 'fulfilled') data[key] = result.value;
  else errors.push(`${label}：${friendlyError(result.reason)}`);
 });
 return { data, errors };
}

export function inviteStatus(invite: { status: string; use_count: number; max_uses: number; expires_at?: string }, now = Date.now()) {
 if (invite.status !== 'active') return '已吊销';
 if (invite.expires_at && new Date(invite.expires_at).getTime() <= now) return '已过期';
 return invite.use_count >= invite.max_uses ? '已用完' : '可用';
}
