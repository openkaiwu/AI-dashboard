import 'package:flutter/material.dart';
import 'api.dart';
import 'store.dart';

/// Manual data is stored as user_manual by the server and never replaces a
/// connector snapshot. It is useful when a provider cannot expose a quota.
class ManualAccountSheet extends StatefulWidget {
  final HubApi api;
  final String providerSlug;
  final VoidCallback onSaved;
  const ManualAccountSheet(
      {super.key,
      required this.api,
      required this.providerSlug,
      required this.onSaved});

  @override
  State<ManualAccountSheet> createState() => _ManualAccountSheetState();
}

class _ManualAccountSheetState extends State<ManualAccountSheet> {
  final name = TextEditingController();
  final plan = TextEditingController();
  final total = TextEditingController();
  final remaining = TextEditingController();
  final unit = TextEditingController(text: 'credit');
  final note = TextEditingController();
  String quotaType = 'credit';
  DateTime? resetAt;
  bool busy = false;
  String? error;

  @override
  void dispose() {
    for (final controller in [name, plan, total, remaining, unit, note]) {
      controller.dispose();
    }
    super.dispose();
  }

  Future<void> chooseReset() async {
    final date = await showDatePicker(
        context: context,
        initialDate: resetAt ?? DateTime.now(),
        firstDate: DateTime(2020),
        lastDate: DateTime(2100));
    if (date == null || !mounted) return;
    final time = await showTimePicker(
        context: context,
        initialTime: TimeOfDay.fromDateTime(resetAt ?? DateTime.now()));
    if (time == null || !mounted) return;
    setState(() => resetAt =
        DateTime(date.year, date.month, date.day, time.hour, time.minute));
  }

  Future<void> save() async {
    final displayName = name.text.trim();
    final limit =
        total.text.trim().isEmpty ? null : double.tryParse(total.text.trim());
    final left = remaining.text.trim().isEmpty
        ? null
        : double.tryParse(remaining.text.trim());
    if (displayName.isEmpty ||
        (total.text.trim().isNotEmpty && limit == null) ||
        (remaining.text.trim().isNotEmpty && left == null) ||
        (limit != null && (limit < 0 || !limit.isFinite)) ||
        (left != null && (left < 0 || !left.isFinite)) ||
        (limit != null && left != null && left > limit)) {
      setState(() => error = '请填写账户名称和有效额度；剩余量不能超过总额度。');
      return;
    }
    setState(() {
      busy = true;
      error = null;
    });
    try {
      final providers = await widget.api.call('GET', '/api/v1/providers');
      final matches = List<Json>.from(providers['providers'] as List? ?? [])
          .where((p) => p['slug'] == widget.providerSlug)
          .toList();
      if (matches.isEmpty) throw StateError('服务器没有此平台');
      await widget.api.call('POST', '/api/v1/provider-accounts', {
        'provider_id': matches.first['id'],
        'display_name': displayName,
        'plan_name': plan.text.trim(),
        'quota': {
          'scope_key': 'manual',
          'quota_type': quotaType,
          'unit': unit.text.trim().isEmpty ? quotaType : unit.text.trim(),
          'limit_value': limit,
          'remaining_value': left,
          'reset_policy': resetAt == null ? 'unknown' : 'fixed_time',
          'reset_at': resetAt?.toUtc().toIso8601String() ?? '',
          'note': note.text.trim(),
        },
      });
      if (!mounted) return;
      widget.onSaved();
      Navigator.pop(context);
    } catch (e) {
      if (mounted) setState(() => error = '保存失败：$e');
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) => SafeArea(
          child: Padding(
        padding: EdgeInsets.fromLTRB(
            20, 16, 20, MediaQuery.viewInsetsOf(context).bottom + 20),
        child: SingleChildScrollView(
            child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
                '手工登记 ${widget.providerSlug == 'cursor' ? 'Cursor' : 'Codex'} 额度',
                style: Theme.of(context).textTheme.titleLarge),
            const Text('数据会标记为手工来源，不覆盖电脑自动采集。'),
            const SizedBox(height: 12),
            TextField(
                controller: name,
                maxLength: 100,
                decoration: const InputDecoration(labelText: '账户名称 *')),
            TextField(
                controller: plan,
                maxLength: 100,
                decoration: const InputDecoration(labelText: '套餐名称')),
            DropdownButtonFormField<String>(
                initialValue: quotaType,
                decoration: const InputDecoration(labelText: '额度类型'),
                items: const [
                  DropdownMenuItem(value: 'credit', child: Text('额度 / credit')),
                  DropdownMenuItem(value: 'token', child: Text('Token')),
                  DropdownMenuItem(value: 'request', child: Text('请求')),
                  DropdownMenuItem(value: 'message', child: Text('消息')),
                  DropdownMenuItem(value: 'currency', child: Text('金额')),
                ],
                onChanged: (value) =>
                    setState(() => quotaType = value ?? 'credit')),
            TextField(
                controller: unit,
                maxLength: 40,
                decoration: const InputDecoration(labelText: '单位')),
            Row(children: [
              Expanded(
                  child: TextField(
                      controller: total,
                      keyboardType:
                          const TextInputType.numberWithOptions(decimal: true),
                      decoration: const InputDecoration(labelText: '总额度'))),
              const SizedBox(width: 12),
              Expanded(
                  child: TextField(
                      controller: remaining,
                      keyboardType:
                          const TextInputType.numberWithOptions(decimal: true),
                      decoration: const InputDecoration(labelText: '剩余额度'))),
            ]),
            TextButton(
                onPressed: chooseReset,
                child: Text(resetAt == null ? '设置重置时间' : '重置于 $resetAt')),
            TextField(
                controller: note,
                maxLength: 300,
                decoration: const InputDecoration(labelText: '来源备注')),
            if (error != null)
              Text(error!, style: const TextStyle(color: Colors.red)),
            FilledButton(
                onPressed: busy ? null : save,
                child: Text(busy ? '保存中…' : '保存到当前账户')),
          ],
        )),
      ));
}
