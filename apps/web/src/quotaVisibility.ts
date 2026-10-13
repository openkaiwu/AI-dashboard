// Match the server's presentation policy without changing captured quota data.
export const showsFiveHour = (plan: string) => plan === 'plus';

export function visibleQuotaWindows<T extends {duration_minutes: number}>(windows: T[], plan: string): T[] {
  return windows.filter(window => window.duration_minutes !== 300 || showsFiveHour(plan));
}
