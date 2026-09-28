import 'dart:async';
import 'package:flutter/material.dart';
import 'api.dart';
import 'theme.dart';

class CursorPage extends StatefulWidget {
  final HubApi api;
  const CursorPage({super.key, required this.api});
  @override
  State<CursorPage> createState() => _CursorPageState();
}

class _CursorPageState extends State<CursorPage> {
  List<Json> accounts = [];
  String? error;
  bool busy = false;
  Timer? timer;

  @override
  void initState() {
    super.initState();
    unawaited(refresh());
    timer = Timer.periodic(const Duration(minutes: 5), (_) => unawaited(refresh()));
  }

  @override
  void dispose() {
    timer?.cancel();
    super.dispose();
  }

  Future<void> refresh() async {
    if (busy || !mounted) return;
    setState(() => busy = true);
    try {
      final data = await widget.api.call('GET', '/api/v1/dashboard');
      final all = List<Json>.from(data['accounts'] as List? ?? []);
      if (mounted) {
        setState(() {
          accounts = all.where((a) => (a['provider'] as Json?)?['slug'] == 'cursor').toList();
          error = null;
        });
      }
    } catch (_) {
      if (mounted) setState(() => error = 'Cursor 额度暂不可用。请在电脑启动采集或手工登记。');
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  String remain(Json account) {
    final buckets = account['buckets'] as List?;
    if (buckets == null || buckets.isEmpty) return '尚未录入';
    if (buckets.length > 1) {
      final parts = <String>[];
      for (final raw in buckets) {
        final b = raw as Json;
        final key = b['scope_key'] as String? ?? 'usage';
        final label = key == 'cursor_models'
            ? 'Cursor'
            : key == 'other_models'
                ? 'Other'
                : key;
        final ratio = b['remaining_ratio'];
        if (ratio is num) {
          parts.add('$label ${((1 - ratio) * 100).round()}%');
        }
      }
      if (parts.isNotEmpty) return parts.join(' · ');
    }
    final b = buckets.first as Json;
    final ratio = b['remaining_ratio'];
    if (ratio is num) return '${((1 - ratio) * 100).round()}% 已用';
    final remain = b['remaining_value'];
    if (remain is num) return remain.toString();
    return '—';
  }

  @override
  Widget build(BuildContext context) {
    return RefreshIndicator(
      onRefresh: refresh,
      child: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          Text('Cursor 额度', style: Theme.of(context).textTheme.headlineSmall),
          const SizedBox(height: 8),
          const Text('自动采集或手工登记。采集失败时不会显示臆造数字。'),
          if (error != null) ...[
            const SizedBox(height: 12),
            Text(error!, style: const TextStyle(color: HubTheme.warn)),
          ],
          const SizedBox(height: 20),
          if (accounts.isEmpty)
            const Card(
              child: Padding(
                padding: EdgeInsets.all(18),
                child: Text('还没有 Cursor 数据。请在电脑登录 Cursor 并启动 bridge 采集程序。'),
              ),
            )
          else
            for (final a in accounts)
              Card(
                child: ListTile(
                  title: Text(a['display_name']?.toString() ?? 'Cursor'),
                  subtitle: Text('${(a['provider'] as Json?)?['display_name'] ?? 'Cursor'} · ${a['computed_status'] ?? 'unknown'}'),
                  trailing: Text(remain(a), style: Theme.of(context).textTheme.titleMedium),
                ),
              ),
        ],
      ),
    );
  }
}
