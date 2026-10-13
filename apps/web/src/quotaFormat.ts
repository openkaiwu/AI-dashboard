export function remainingLabel(value: number) {
  if (!Number.isFinite(value)) return '未知';
  if (value > 0 && value < 0.1) return '<0.1';
  return value > 0 && value < 1 ? value.toFixed(1) : value.toFixed(0);
}
