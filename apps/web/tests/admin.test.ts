import { it, expect, vi } from 'vitest';
vi.mock('../src/session',()=>({}));
import { loadAdminSections, inviteStatus } from '../src/adminData';

it('reports failed invite and review endpoints without hiding successful sections', async () => {
 const result = await loadAdminSections(async path => {
  if (path.includes('invites')) throw new Error('邀请码服务暂不可用');
  if (path.includes('registrations')) throw new Error('审核服务暂不可用');
  return { users: [{ id: 'owner' }] };
 });
 expect(result.data.users.users[0].id).toBe('owner');
 expect(result.errors).toEqual(['注册审核：请求失败，请稍后重试。', '邀请码：请求失败，请稍后重试。']);
});

it('includes reviewed registrations and clears failures after recovery', async () => {
 const paths: string[] = [];
 const result = await loadAdminSections(async path => { paths.push(path); return {}; });
 expect(paths).toContain('/api/v1/admin/registrations?status=all');
 expect(result.errors).toEqual([]);
});

it('distinguishes expired, exhausted and revoked invitations', () => {
 const invite = { status: 'active', use_count: 0, max_uses: 1, expires_at: '2026-10-06T00:00:00Z' };
 const now = Date.parse('2026-10-07T00:00:00Z');
 expect(inviteStatus(invite, now)).toBe('已过期');
 expect(inviteStatus({ ...invite, status: 'revoked' }, now)).toBe('已吊销');
 expect(inviteStatus({ status: 'active', use_count: 1, max_uses: 1 }, now)).toBe('已用完');
 expect(inviteStatus({ status: 'active', use_count: 0, max_uses: 1 }, now)).toBe('可用');
});
