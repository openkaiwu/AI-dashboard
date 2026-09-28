import 'dart:async';
import 'package:flutter/material.dart';
import 'api.dart';
import 'store.dart';
import 'theme.dart';

class CodexPage extends StatefulWidget {
  final HubApi api;
  final HubStore store;
  final String scope;
  const CodexPage(
      {super.key, required this.api, required this.store, required this.scope});
  @override
  State<CodexPage> createState() => _CodexPageState();
}

class _CodexPageState extends State<CodexPage>
    with SingleTickerProviderStateMixin {
  Json? overview;
  String? error;
  bool busy = false;
  Timer? timer;
  late TabController tabs;
  String selectedDevice = '';
  String reminderFilter = 'pending';

  @override
  void initState() {
    super.initState();
    tabs = TabController(length: 5, vsync: this);
    unawaited(load());
    timer = Timer.periodic(
        const Duration(minutes: 5), (_) => unawaited(refresh()));
  }

  @override
  void dispose() {
    timer?.cancel();
    tabs.dispose();
    super.dispose();
  }

  Future<void> load() async {
    try {
      final cached = await widget.store.readCodex('${widget.scope}:advisor');
      if (mounted && cached.isNotEmpty) {
        setState(() => overview = cached.first as Json);
      }
    } catch (_) {}
    await refresh();
  }

  Future<void> refresh() async {
    if (busy || !mounted) return;
    setState(() => busy = true);
    try {
      final data = await widget.api.call('GET', '/api/v1/codex/overview');
      await widget.store.saveCodex('${widget.scope}:advisor', [data]);
      if (mounted) {
        setState(() {
          overview = data;
          error = null;
          if (selectedDevice.isEmpty &&
              (data['devices'] as List?)?.isNotEmpty == true) {
            selectedDevice = data['devices'][0]['id'] as String;
          }
        });
      }
    } catch (_) {
      if (mounted) {
        setState(() => error = '服务暂不可用，显示上次缓存。');
      }
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Json? device() {
    final items = overview?['devices'] as List?;
    if (items == null || items.isEmpty) return null;
    return items.cast<Json>().firstWhere(
          (d) => d['id'] == selectedDevice,
          orElse: () => items.first as Json,
        );
  }

  bool stale(Json? d) {
    if (error != null || d == null) return true;
    final snap = d['snapshot'] as Json?;
    if (snap == null || snap['status'] != 'ok') return true;
    final observed = DateTime.tryParse(snap['observed_at'] as String? ?? '');
    if (observed == null) return true;
    return DateTime.now().difference(observed).inMinutes >= 11;
  }

  Future<void> alertAction(String id, String action) async {
    try {
      await widget.api.call(
          'POST',
          '/api/v1/codex/alerts/${Uri.encodeComponent(id)}',
          {'action': action});
      await refresh();
    } catch (_) {
      if (mounted) setState(() => error = '暂时无法更新提醒');
    }
  }

  Future<void> toggleReminders(bool enabled) async {
    try {
      await widget.api.call('PATCH', '/api/v1/codex/preferences',
          {...overview!['preferences'] as Json, 'enabled': enabled});
      await refresh();
    } catch (_) {
      if (mounted) setState(() => error = '规则保存失败');
    }
  }

  String when(dynamic value) => value == null
      ? '未知'
      : DateTime.parse(value as String).toLocal().toString().split('.').first;

  String hours(num? value) {
    if (value == null) return '时间未知';
    if (value <= 0) return '等待重置确认';
    if (value >= 24) return '${(value / 24).toStringAsFixed(1)} 天';
    return '${value.toStringAsFixed(1)} 小时';
  }

  Widget eyebrow(String text) => Padding(
        padding: const EdgeInsets.only(bottom: 8),
        child: Text(text.toUpperCase(),
            style: const TextStyle(
                fontSize: 11,
                letterSpacing: 1.2,
                color: HubTheme.muted,
                fontWeight: FontWeight.w600)),
      );

  Widget hero(Json? d, bool isStale) {
    final state = isStale ? 'stale' : (d?['analysis']?['state'] ?? 'unknown');
    final advice = (d?['analysis']?['advice'] as List?)?.cast<Json>() ?? [];
    final top = advice.where((a) => a['kind'] != 'news').toList()
      ..sort((a, b) => (b['priority'] as num).compareTo(a['priority'] as num));
    final labels = {
      'slow': '建议放慢',
      'use': '适合多用',
      'balanced': '节奏平稳',
      'stale': '等待新数据',
      'unknown': '信息不足',
    };
    final explain = {
      'slow': '保留后续任务的空间，先完成优先级最高的工作。',
      'use': '把计划中的任务提前，充分使用本轮额度。',
      'balanced': '额度与时间暂未触发提醒，按自己的工作节奏继续。',
      'stale': '电脑可能离线或额度暂不可读，暂停节奏建议。',
      'unknown': '额度窗口信息尚不完整，不推算可用余量。',
    };
    final color = state == 'slow'
        ? const Color(0xfffff5e5)
        : state == 'stale'
            ? HubTheme.bg2
            : const Color(0xfff0f6e9);
    final border = state == 'slow'
        ? const Color(0xffead7b4)
        : state == 'stale'
            ? HubTheme.line
            : const Color(0xffd6e3cb);
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(22),
      decoration: BoxDecoration(
        color: color,
        borderRadius: BorderRadius.circular(22),
        border: Border.all(color: border),
      ),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Text('现在的建议 · ${labels[state] ?? state}',
            style: TextStyle(
                fontSize: 12,
                color: state == 'slow' ? HubTheme.warn : HubTheme.accent)),
        const SizedBox(height: 10),
        Text(top.isNotEmpty ? top.first['title'] : labels[state],
            style: Theme.of(context).textTheme.headlineSmall),
        const SizedBox(height: 8),
        Text(top.isNotEmpty ? top.first['body'] : explain[state],
            style: Theme.of(context).textTheme.bodyMedium),
      ]),
    );
  }

  Widget metricTile(Json m) {
    final remaining = (m['remaining'] as num).toDouble();
    final minutes = m['duration_minutes'] as int? ?? 0;
    final title = minutes % 1440 == 0
        ? '${minutes ~/ 1440} 天额度'
        : '${minutes / 60} 小时额度';
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [
            Text(title, style: Theme.of(context).textTheme.labelSmall),
            Text(m['bucket'] as String? ?? '',
                style: Theme.of(context).textTheme.labelSmall),
          ]),
          const SizedBox(height: 18),
          Text('${remaining.toStringAsFixed(0)}%',
              style: const TextStyle(
                  fontSize: 52,
                  height: 1,
                  letterSpacing: -2,
                  color: HubTheme.ink)),
          const Text('% 剩余', style: TextStyle(color: HubTheme.muted)),
          const SizedBox(height: 14),
          ClipRRect(
            borderRadius: BorderRadius.circular(99),
            child: LinearProgressIndicator(
              value: remaining / 100,
              minHeight: 6,
              backgroundColor: HubTheme.line,
              color: HubTheme.accent,
            ),
          ),
          const SizedBox(height: 14),
          Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [
            const Text('距离重置', style: TextStyle(color: HubTheme.muted)),
            Text(hours(m['hours_left'] as num?)),
          ]),
          const SizedBox(height: 8),
          Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [
            const Text('均匀使用预算', style: TextStyle(color: HubTheme.muted)),
            Text(m['daily_budget'] == null
                ? '暂不可估'
                : '${(m['daily_budget'] as num).toStringAsFixed(1)}% / 天'),
          ]),
        ]),
      ),
    );
  }

  Widget creditTile(Json? snap) {
    final credits = snap?['reset_credits'] as Json?;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [
            Text('备用重置卡', style: TextStyle(color: HubTheme.muted)),
            Text('↺', style: TextStyle(color: HubTheme.accent)),
          ]),
          const SizedBox(height: 16),
          Text(credits == null ? '—' : '${credits['available_count']}',
              style: const TextStyle(
                  fontSize: 48, color: HubTheme.accent, height: 1)),
          Text(credits == null ? '状态未知' : '张可用',
              style: const TextStyle(color: HubTheme.muted)),
          const SizedBox(height: 12),
          if (credits == null)
            const Text('Codex 尚未返回重置卡信息。')
          else if ((credits['available_count'] as num) == 0)
            const Text('当前没有可用的重置卡。')
          else
            const Text('接近到期时会单独提醒。只提醒，不自动兑换。'),
          if (credits?['items'] != null)
            for (final item in credits!['items'] as List)
              Padding(
                padding: const EdgeInsets.only(top: 10),
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.spaceBetween,
                  children: [
                    const Text('重置卡'),
                    Text(item['expires_at'] == null
                        ? '到期时间未知'
                        : '${DateTime.fromMillisecondsSinceEpoch((item['expires_at'] as int) * 1000).toLocal()} 到期'),
                  ],
                ),
              ),
        ]),
      ),
    );
  }

  Widget overviewTab(Json? d, bool isStale) {
    if (overview?['devices'] == null ||
        (overview!['devices'] as List).isEmpty) {
      return Card(
        child: Padding(
          padding: const EdgeInsets.all(28),
          child: Column(children: [
            const Icon(Icons.link_off, size: 42, color: HubTheme.accent),
            const SizedBox(height: 16),
            Text('先连接电脑上的 Codex',
                style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 8),
            const Text('在网页版「电脑采集器」生成配置，并在电脑启动采集程序。'),
          ]),
        ),
      );
    }
    final metrics =
        (d?['analysis']?['metrics'] as List?)?.cast<Json>() ?? [];
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      hero(d, isStale),
      const SizedBox(height: 16),
      for (final m in metrics) metricTile(m),
      creditTile(d?['snapshot'] as Json?),
    ]);
  }

  Widget planTab(Json? d) {
    final metrics = (d?['analysis']?['metrics'] as List?)?.cast<Json>() ?? [];
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Text('使用计划', style: Theme.of(context).textTheme.titleMedium),
      const Text('按已观测的额度与重置时间估算；数据不足时不推断。'),
      for (final m in metrics) Card(child: Padding(padding: const EdgeInsets.all(16), child: Column(
        crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text('${m['bucket']} · ${(m['duration_minutes'] as num) / 60} 小时额度'),
          Text('剩余 ${(m['remaining'] as num).toStringAsFixed(0)}% · 距重置 ${hours(m['hours_left'] as num?)}'),
          Text(m['daily_budget'] == null ? '暂无可靠的每日预算' : '均匀使用参考：每天 ${(m['daily_budget'] as num).toStringAsFixed(1)}%'),
        ],
      ))),
      if (metrics.isEmpty) const Text('暂无可制定计划的额度窗口。'),
    ]);
  }

  Widget radarTab(Json? d) {
    final metrics = (d?['analysis']?['metrics'] as List?)?.cast<Json>() ?? [];
    final news = d?['snapshot']?['news'] as Json?;
    final items = (news?['items'] as List?)?.cast<Json>() ?? [];
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Text('重置雷达', style: Theme.of(context).textTheme.titleMedium),
      const Text('账户重置以本机采集到的窗口为准；外部消息仅供参考。'),
      for (final m in metrics.where((m) => m['resets_at'] != null))
        ListTile(title: Text('${m['bucket']} · ${(m['duration_minutes'] as num) / 60} 小时'),
          subtitle: Text('重置于 ${DateTime.fromMillisecondsSinceEpoch((m['resets_at'] as num).toInt() * 1000).toLocal()}')),
      if (metrics.every((m) => m['resets_at'] == null)) const Text('暂无可核实的重置时间。'),
      for (final item in items) Card(child: Padding(padding: const EdgeInsets.all(14), child: Text(item['summary']?.toString() ?? ''))),
    ]);
  }

  String alertState(Json a) {
    if (a['dismissed'] == true) return 'dismissed';
    final snooze = a['snoozed_until'];
    if (snooze != null &&
        DateTime.parse(snooze as String).isAfter(DateTime.now())) {
      return 'snoozed';
    }
    return 'pending';
  }

  Widget remindersTab() {
    final alerts =
        (overview?['alerts'] as List?)?.cast<Json>() ?? [];
    final visible =
        alerts.where((a) => alertState(a) == reminderFilter).toList();
    final pending =
        alerts.where((a) => alertState(a) == 'pending').length;
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Row(children: [
        Text('值得留意', style: Theme.of(context).textTheme.titleMedium),
        const Spacer(),
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
          decoration: BoxDecoration(
              color: HubTheme.accentDim,
              borderRadius: BorderRadius.circular(6)),
          child: Text('$pending 条提醒',
              style: const TextStyle(fontSize: 11, color: HubTheme.accent)),
        ),
      ]),
      const SizedBox(height: 8),
      Text(overview?['quiet'] == true
          ? '当前免打扰，站内仍展示。'
          : '提醒已去重；可稍后 1 小时提醒，或忽略本次事件。'),
      const SizedBox(height: 12),
      Wrap(spacing: 8, children: [
        for (final entry in [
          ['pending', '待处理'],
          ['snoozed', '稍后提醒'],
          ['dismissed', '已忽略'],
        ])
          FilterChip(
            label: Text('${entry[1]} · ${alerts.where((a) => alertState(a) == entry[0]).length}'),
            selected: reminderFilter == entry[0],
            onSelected: (_) => setState(() => reminderFilter = entry[0]),
          ),
      ]),
      const SizedBox(height: 12),
      if (visible.isEmpty)
        const Card(
          child: Padding(
            padding: EdgeInsets.all(24),
            child: Text('此分类暂无提醒'),
          ),
        ),
      for (final a in visible)
        Card(
          child: Padding(
            padding: const EdgeInsets.all(18),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(a['advice']['title'],
                    style: Theme.of(context).textTheme.titleMedium),
                const SizedBox(height: 8),
                Text(a['advice']['body']),
                const SizedBox(height: 8),
                Wrap(spacing: 8, children: [
                  if (reminderFilter == 'pending') ...[
                    OutlinedButton(
                        onPressed: () => alertAction(a['id'], 'snooze'),
                        child: const Text('1 小时后提醒')),
                    TextButton(
                        onPressed: () => alertAction(a['id'], 'dismiss'),
                        child: const Text('忽略本次')),
                  ] else
                    OutlinedButton(
                        onPressed: () => alertAction(a['id'], 'restore'),
                        child: const Text('恢复到待处理')),
                ]),
              ],
            ),
          ),
        ),
    ]);
  }

  Widget settingsTab() {
    final prefs = overview?['preferences'] as Json?;
    final plan = overview?['plan'] as Json?;
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Text('按你的习惯提醒', style: Theme.of(context).textTheme.titleMedium),
      DropdownButtonFormField<String>(
        initialValue: plan?['plan_type'] as String? ?? 'unknown',
        decoration: const InputDecoration(labelText: 'Codex 套餐（手动设置）'),
        items: const [
          DropdownMenuItem(value: 'unknown', child: Text('未知')),
          DropdownMenuItem(value: 'plus', child: Text('Plus · 显示五小时额度')),
          DropdownMenuItem(value: 'pro', child: Text('Pro · 隐藏五小时额度及提醒')),
        ],
        onChanged: (value) async {
          if (value == null) return;
          try { await widget.api.call('PUT', '/api/v1/codex/plan', {'plan_type': value}); await refresh(); }
          catch (_) { if (mounted) setState(() => error = '套餐设置失败'); }
        },
      ),
      const SizedBox(height: 8),
      const Text('规则跨端保存；手机端暂不支持系统推送，仅站内提醒。'),
      const SizedBox(height: 16),
      if (prefs != null)
        SwitchListTile(
          contentPadding: EdgeInsets.zero,
          title: const Text('启用额度与到期提醒'),
          value: prefs['enabled'] == true,
          onChanged: toggleReminders,
        ),
      const SizedBox(height: 8),
      const Text('更多阈值可在网页版「提醒设置」中调整。'),
    ]);
  }

  @override
  Widget build(BuildContext context) {
    final d = device();
    final isStale = stale(d);
    final devices = (overview?['devices'] as List?)?.cast<Json>() ?? [];
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      eyebrow('Codex / Personal Companion'),
      Text('让额度跟上你的节奏', style: Theme.of(context).textTheme.headlineMedium),
      const SizedBox(height: 6),
      const Text('每 5 分钟接收电脑额度摘要；登录凭据和对话留在电脑。'),
      const SizedBox(height: 16),
      Row(children: [
        Container(
          width: 8,
          height: 8,
          decoration: BoxDecoration(
            color: isStale ? HubTheme.warn : HubTheme.accent,
            shape: BoxShape.circle,
          ),
        ),
        const SizedBox(width: 8),
        Expanded(
          child: Text(
            isStale
                ? '等待电脑新数据'
                : '${d?['name'] ?? '电脑'} · 已同步',
            style: Theme.of(context).textTheme.labelSmall,
          ),
        ),
        if (devices.length > 1)
          DropdownButton<String>(
            value: selectedDevice.isEmpty ? devices.first['id'] : selectedDevice,
            items: [
              for (final item in devices)
                DropdownMenuItem(
                    value: item['id'] as String,
                    child: Text(item['name'] as String)),
            ],
            onChanged: (v) => setState(() => selectedDevice = v ?? ''),
          ),
        TextButton(
            onPressed: busy ? null : refresh,
            child: Text(busy ? '同步中…' : '↻ 刷新')),
      ]),
      if (error != null)
        Padding(
          padding: const EdgeInsets.only(top: 8),
          child: Text(error!, style: const TextStyle(color: HubTheme.warn)),
        ),
      const SizedBox(height: 12),
      TabBar(
        controller: tabs,
        isScrollable: true,
        labelColor: HubTheme.accent,
        unselectedLabelColor: HubTheme.muted,
        indicatorColor: HubTheme.accent,
        tabs: const [
          Tab(text: '额度总览'),
          Tab(text: '使用计划'),
          Tab(text: '提醒中心'),
          Tab(text: '重置雷达'),
          Tab(text: '提醒设置'),
        ],
      ),
      const SizedBox(height: 12),
      Expanded(
        child: TabBarView(
          controller: tabs,
          children: [
            ListView(children: [overviewTab(d, isStale)]),
            ListView(children: [planTab(d)]),
            ListView(children: [remindersTab()]),
            ListView(children: [radarTab(d)]),
            ListView(children: [settingsTab()]),
          ],
        ),
      ),
    ]);
  }
}
