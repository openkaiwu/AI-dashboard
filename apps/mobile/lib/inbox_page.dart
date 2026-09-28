import 'dart:async';
import 'package:flutter/material.dart';
import 'api.dart';
import 'theme.dart';

class InboxPage extends StatefulWidget {
  final HubApi api;
  final String uiMode;
  final ValueChanged<String> onModeSwitch;
  const InboxPage({
    super.key,
    required this.api,
    required this.uiMode,
    required this.onModeSwitch,
  });
  @override
  State<InboxPage> createState() => _InboxPageState();
}

class _InboxPageState extends State<InboxPage> {
  List<Json> items = [];
  String? error;
  bool busy = false;
  Timer? timer;

  @override
  void initState() {
    super.initState();
    unawaited(refresh());
    timer = Timer.periodic(const Duration(minutes: 2), (_) => unawaited(refresh()));
  }

  @override
  void dispose() {
    timer?.cancel();
    super.dispose();
  }

  @override
  void didUpdateWidget(covariant InboxPage oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.uiMode != widget.uiMode) unawaited(refresh());
  }

  bool matchesMode(Json n) {
    final slug = (n['provider_slug'] as String?) ?? '';
    if (widget.uiMode == 'cursor') return slug.isEmpty || slug == 'cursor';
    return slug.isEmpty || slug != 'cursor';
  }

  Future<void> refresh() async {
    if (busy || !mounted) return;
    setState(() => busy = true);
    try {
      final data = await widget.api.call('GET', '/api/v1/notifications');
      final all = List<Json>.from(data['notifications'] as List? ?? []);
      if (mounted) {
        setState(() {
          items = all.where(matchesMode).toList();
          error = null;
        });
      }
    } catch (_) {
      if (mounted) setState(() => error = '通知暂不可用');
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> readAll() async {
    try {
      await widget.api.call('POST', '/api/v1/notifications/read-all');
      await refresh();
    } catch (_) {
      if (mounted) setState(() => error = '暂时无法标记已读');
    }
  }

  Future<void> readOne(String id) async {
    try {
      await widget.api.call('POST', '/api/v1/notifications/${Uri.encodeComponent(id)}/read');
      await refresh();
    } catch (_) {
      if (mounted) setState(() => error = '暂时无法标记已读');
    }
  }

  String when(dynamic value) => value == null
      ? '—'
      : DateTime.parse(value as String).toLocal().toString().split('.').first;

  @override
  Widget build(BuildContext context) {
    return RefreshIndicator(
      onRefresh: refresh,
      child: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          Row(children: [
            Expanded(
              child: Text('通知中心', style: Theme.of(context).textTheme.headlineSmall),
            ),
            TextButton(onPressed: busy ? null : readAll, child: const Text('全部已读')),
          ]),
          const SizedBox(height: 8),
          const Text('同一提醒在条件解除前不会重复轰炸。点击可切换对应模式。'),
          if (error != null) ...[
            const SizedBox(height: 12),
            Text(error!, style: const TextStyle(color: HubTheme.warn)),
          ],
          const SizedBox(height: 20),
          if (items.isEmpty)
            const Card(
              child: Padding(
                padding: EdgeInsets.all(18),
                child: Text('还没有通知。'),
              ),
            )
          else
            for (final n in items)
              Card(
                child: ListTile(
                  title: Text(n['title'] as String? ?? ''),
                  subtitle: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(n['body'] as String? ?? ''),
                      const SizedBox(height: 4),
                      Text(
                        '${n['provider_name'] ?? '—'} · ${n['severity'] ?? ''} · ${when(n['created_at'])}',
                        style: Theme.of(context).textTheme.labelSmall,
                      ),
                    ],
                  ),
                  trailing: n['status'] == 'unread'
                      ? TextButton(
                          onPressed: () => readOne(n['id'] as String),
                          child: const Text('已读'),
                        )
                      : const Text('已读', style: TextStyle(color: HubTheme.muted)),
                  onTap: () {
                    final slug = n['provider_slug'] as String?;
                    if (slug == 'cursor' || slug == 'codex') {
                      widget.onModeSwitch(slug!);
                    }
                    if (n['status'] == 'unread') unawaited(readOne(n['id'] as String));
                  },
                ),
              ),
        ],
      ),
    );
  }
}
