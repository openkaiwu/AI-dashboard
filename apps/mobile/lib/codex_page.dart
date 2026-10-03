import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
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
  double reservePercent = 10;

  @override
  void initState() {
    super.initState();
    tabs = TabController(length: 5, vsync: this);
    unawaited(load());
    timer =
        Timer.periodic(const Duration(minutes: 5), (_) => unawaited(refresh()));
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
    final title =
        minutes % 1440 == 0 ? '${minutes ~/ 1440} 天额度' : '${minutes / 60} 小时额度';
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
          const SizedBox(height: 8),
          Text(
              m['rate_per_hour'] == null
                  ? '样本不足，暂不预测耗尽时间'
                  : '近期消耗 ${(m['rate_per_hour'] as num).toStringAsFixed(1)} 个百分点 / 小时',
              style: Theme.of(context).textTheme.labelSmall),
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
          const Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
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
    final metrics = (d?['analysis']?['metrics'] as List?)?.cast<Json>() ?? [];
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      hero(d, isStale),
      const SizedBox(height: 16),
      for (final m in metrics) metricTile(m),
      creditTile(d?['snapshot'] as Json?),
    ]);
  }

  Widget planTab(Json? d, bool isStale) {
    final metrics = (d?['analysis']?['metrics'] as List?)?.cast<Json>() ?? [];
    final credits =
        (d?['snapshot']?['reset_credits']?['items'] as List?)?.cast<Json>() ??
            [];
    final events = <({String title, int at})>[
      for (final m in metrics)
        if (m['resets_at'] is num)
          (
            title:
                '${m['bucket']} · ${(m['duration_minutes'] as num) / 60} 小时窗口重置',
            at: (m['resets_at'] as num).toInt()
          ),
      for (final c in credits)
        if (c['expires_at'] is num)
          (title: '重置卡到期', at: (c['expires_at'] as num).toInt()),
    ]..sort((a, b) => a.at.compareTo(b.at));
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Text('使用计划', style: Theme.of(context).textTheme.titleMedium),
      const Text('按已观测的额度与重置时间估算；数据不足时不推断。预留值只用于本次试算。'),
      const SizedBox(height: 12),
      Text('预留额度：${reservePercent.round()} 个百分点'),
      Slider(
          value: reservePercent,
          min: 0,
          max: 50,
          divisions: 10,
          label: '${reservePercent.round()}%',
          onChanged: (v) => setState(() => reservePercent = v)),
      for (final m in metrics)
        Card(
            child: Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                        '${m['bucket']} · ${(m['duration_minutes'] as num) / 60} 小时额度'),
                    Text(
                        '剩余 ${(m['remaining'] as num).toStringAsFixed(0)}% · 距重置 ${hours(m['hours_left'] as num?)}'),
                    Builder(builder: (_) {
                      final left = m['hours_left'] as num?;
                      if (isStale || left == null || left <= 0) {
                        return const Text('等待有效数据，暂不计算预算。');
                      }
                      final available =
                          ((m['remaining'] as num).toDouble() - reservePercent)
                              .clamp(0.0, 100.0);
                      final nextDay = available * (24 / left).clamp(0.0, 1.0);
                      return Text(
                          '本轮可分配 ${available.toStringAsFixed(1)} 个百分点 · 未来 24 小时最多 ${nextDay.toStringAsFixed(1)} 个百分点');
                    }),
                  ],
                ))),
      if (metrics.isEmpty) const Text('暂无可制定计划的额度窗口。'),
      const SizedBox(height: 16),
      Text('重置与到期时间线', style: Theme.of(context).textTheme.titleMedium),
      if (isStale) const Text('快照已过期，以下仅为最后已知时间。'),
      if (events.isEmpty) const Text('尚未返回明确的重置或到期时间。'),
      for (final event in events)
        ListTile(
          title: Text(event.title),
          subtitle: Text(DateTime.fromMillisecondsSinceEpoch(event.at * 1000)
              .toLocal()
              .toString()
              .split('.')
              .first),
          trailing: event.at * 1000 <= DateTime.now().millisecondsSinceEpoch
              ? const Text('待采集确认')
              : null,
        ),
      Text('当前电脑最近采集 ${(d?['history'] as List?)?.length ?? 0} 次。历史导出请在桌面端操作。'),
    ]);
  }

  Future<void> editPreference(
      String key, String label, int min, int max) async {
    final prefs = overview?['preferences'] as Json?;
    if (prefs == null) return;
    final value = await showDialog<int>(
      context: context,
      builder: (_) => _PreferenceDialog(
          label: label, initialValue: '${prefs[key]}', min: min, max: max),
    );
    if (value == null || !mounted) return;
    try {
      await widget.api
          .call('PATCH', '/api/v1/codex/preferences', {...prefs, key: value});
      await refresh();
    } catch (_) {
      if (mounted) setState(() => error = '规则保存失败');
    }
  }

  Future<void> openOriginal(String raw) async {
    final uri = Uri.tryParse(raw);
    if (uri == null ||
        uri.scheme != 'https' ||
        uri.host != 'x.com' ||
        !RegExp(r'^/thsottiaux/status/\d+$').hasMatch(uri.path)) {
      return;
    }
    try {
      await const MethodChannel('dev.aihub/mobile_links')
          .invokeMethod<bool>('openOriginal', {'url': uri.toString()});
    } on PlatformException {
      if (mounted) setState(() => error = '无法打开原帖');
    } on MissingPluginException {
      if (mounted) setState(() => error = '当前平台不支持打开原帖');
    }
  }

  Widget radarTab(Json? d, Json? overview) {
    final metrics = (d?['analysis']?['metrics'] as List?)?.cast<Json>() ?? [];
    final news = (d?['snapshot']?['news'] ?? overview?['news']) as Json?;
    final items = (news?['items'] as List?)?.cast<Json>() ?? [];
    final checkedAt = DateTime.tryParse(news?['checked_at']?.toString() ?? '');
    final expired =
        checkedAt == null || DateTime.now().difference(checkedAt).inMinutes >= 45;
    final status = news == null
        ? '尚未检查'
        : expired
            ? '检查记录已过期'
            : news['status'] == 'checked'
                ? '已完成本次检查'
                : '本次无法确认';
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Text('重置雷达', style: Theme.of(context).textTheme.titleMedium),
      const Text('账户重置以本机采集到的窗口为准；外部消息仅供参考。'),
      for (final m in metrics.where((m) => m['resets_at'] != null))
        ListTile(
            title: Text(
                '${m['bucket']} · ${(m['duration_minutes'] as num) / 60} 小时'),
            subtitle: Text(
                '重置于 ${DateTime.fromMillisecondsSinceEpoch((m['resets_at'] as num).toInt() * 1000).toLocal()}')),
      if (metrics.every((m) => m['resets_at'] == null))
        const Text('暂无可核实的重置时间。'),
      const SizedBox(height: 16),
      Text('Tibo 重置消息 · 自动检查',
          style: Theme.of(context).textTheme.titleMedium),
      Text(status,
          style: TextStyle(
              color: expired || news?['status'] != 'checked'
                  ? HubTheme.warn
                  : HubTheme.accent)),
      Text(news == null ? '等待电脑同步检查结果' : '上次尝试 ${when(news['checked_at'])}'),
      for (final item in items)
        Card(
            child: Padding(
                padding: const EdgeInsets.all(14),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                        '${item['approximate_time'] == true ? '约 ' : ''}${when(item['published_at'])}',
                        style: Theme.of(context).textTheme.labelSmall),
                    const SizedBox(height: 6),
                    Text(item['summary']?.toString() ?? ''),
                    TextButton(
                        onPressed: () =>
                            openOriginal(item['url']?.toString() ?? ''),
                        child: const Text('查看原帖 ↗')),
                  ],
                ))),
      if (news?['status'] == 'checked' && items.isEmpty)
        const Text('本次未发现可确认的新重置消息。'),
      const Text('外部消息不会修改账户额度或重置时间；无法确认不等于没有消息。'),
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
    final alerts = (overview?['alerts'] as List?)?.cast<Json>() ?? [];
    final visible =
        alerts.where((a) => alertState(a) == reminderFilter).toList();
    final pending = alerts.where((a) => alertState(a) == 'pending').length;
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
            label: Text(
                '${entry[1]} · ${alerts.where((a) => alertState(a) == entry[0]).length}'),
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
          DropdownMenuItem(value: 'pro', child: Text('Pro · 显示五小时额度（上限更高）')),
        ],
        onChanged: (value) async {
          if (value == null) return;
          try {
            await widget.api
                .call('PUT', '/api/v1/codex/plan', {'plan_type': value});
            await refresh();
          } catch (_) {
            if (mounted) setState(() => error = '套餐设置失败');
          }
        },
      ),
      const SizedBox(height: 8),
      const Text('规则跨端保存。强提醒在手机上以弹窗显示；应用在后台时从屏幕顶部弹出。'),
      const SizedBox(height: 16),
      if (prefs != null)
        SwitchListTile(
          contentPadding: EdgeInsets.zero,
          title: const Text('启用额度与到期提醒'),
          value: prefs['enabled'] == true,
          onChanged: toggleReminders,
        ),
      const SizedBox(height: 8),
      if (overview?['quiet'] == true) const Text('当前处于免打扰时段，站内提醒仍可查看。'),
      if (prefs != null)
        for (final field in const [
          ('near_hours', '临近重置 · 小时', 1, 168),
          ('spare_percent', '多用提醒 · 剩余至少 %', 10, 95),
          ('low_percent', '低额度提醒 · 剩余不超过 %', 1, 30),
          ('pace_lead', '超前消耗 · 百分点', 5, 70),
          ('credit_hours', '重置卡到期前 · 小时', 1, 168),
          ('cooldown_hours', '重复提醒间隔 · 小时', 1, 48),
          ('quiet_start', '免打扰开始 · 当地时', 0, 23),
          ('quiet_end', '免打扰结束 · 当地时', 0, 23),
          ('utc_offset_minutes', '时区 · UTC 偏移分钟', -720, 840),
        ])
          ListTile(
            contentPadding: EdgeInsets.zero,
            title: Text(field.$2),
            subtitle: Text('${prefs[field.$1]}'),
            trailing: const Icon(Icons.edit_outlined),
            onTap: () => editPreference(field.$1, field.$2, field.$3, field.$4),
          ),
      const Text('规则保存在账户中，两端同步。'),
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
            isStale ? '等待电脑新数据' : '${d?['name'] ?? '电脑'} · 已同步',
            style: Theme.of(context).textTheme.labelSmall,
          ),
        ),
        if (devices.length > 1)
          DropdownButton<String>(
            value:
                selectedDevice.isEmpty ? devices.first['id'] : selectedDevice,
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
      if (overview?['plan']?['plan_type'] == 'unknown')
        const Text('Codex 套餐尚未确认，请在提醒设置中选择 Plus 或 Pro。'),
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
            ListView(children: [planTab(d, isStale)]),
            ListView(children: [remindersTab()]),
            ListView(children: [radarTab(d, overview)]),
            ListView(children: [settingsTab()]),
          ],
        ),
      ),
    ]);
  }
}

class _PreferenceDialog extends StatefulWidget {
  final String label;
  final String initialValue;
  final int min;
  final int max;
  const _PreferenceDialog(
      {required this.label,
      required this.initialValue,
      required this.min,
      required this.max});

  @override
  State<_PreferenceDialog> createState() => _PreferenceDialogState();
}

class _PreferenceDialogState extends State<_PreferenceDialog> {
  late final TextEditingController controller;

  @override
  void initState() {
    super.initState();
    controller = TextEditingController(text: widget.initialValue);
  }

  @override
  void dispose() {
    controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
        title: Text(widget.label),
        content: TextField(
          controller: controller,
          autofocus: true,
          keyboardType: const TextInputType.numberWithOptions(signed: true),
          inputFormatters: [
            FilteringTextInputFormatter.allow(RegExp(r'[-0-9]'))
          ],
          decoration:
              InputDecoration(helperText: '允许范围：${widget.min} 至 ${widget.max}'),
        ),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(context), child: const Text('取消')),
          FilledButton(
              onPressed: () {
                final parsed = int.tryParse(controller.text);
                if (parsed == null ||
                    parsed < widget.min ||
                    parsed > widget.max) {
                  return;
                }
                Navigator.pop(context, parsed);
              },
              child: const Text('保存')),
        ],
      );
}
