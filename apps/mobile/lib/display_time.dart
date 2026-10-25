String formatLocalTimestamp(Object? value, {String empty = '暂无记录'}) {
  final parsed = value is String ? DateTime.tryParse(value) : null;
  if (parsed == null) return empty;
  final time = parsed.toLocal();
  String two(int n) => n.toString().padLeft(2, '0');
  return '${two(time.month)}-${two(time.day)} ${two(time.hour)}:${two(time.minute)}';
}
