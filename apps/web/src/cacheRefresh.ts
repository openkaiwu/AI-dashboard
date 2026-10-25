// Cache errors must not discard a successful network response.
export async function refreshWithCache<T>(fetch: () => Promise<T>, save: (data: T) => void | Promise<void>) {
  const data = await fetch();
  try {
    await save(data);
    return {data, cacheError: ''};
  } catch {
    return {data, cacheError: '最新数据已显示，但本机缓存保存失败，请检查可用空间。'};
  }
}

export function localTimestamp(value?: string | null, empty = '暂无记录') {
  const date = value ? new Date(value) : null;
  return date && Number.isFinite(date.getTime())
    ? date.toLocaleString([], {month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'})
    : empty;
}
