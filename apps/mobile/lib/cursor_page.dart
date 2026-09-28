import 'dart:async';
import 'package:flutter/material.dart';
import 'api.dart';
import 'store.dart';
import 'theme.dart';

class CursorPage extends StatefulWidget {
  final HubApi api;
  const CursorPage({super.key, required this.api});
  @override
  State<CursorPage> createState() => _CursorPageState();
}

class _CursorPageState extends State<CursorPage> {
  List<Json> accounts = [], notifications = [], rules = [];
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
  void dispose() { timer?.cancel(); super.dispose(); }

  Future<void> refresh() async {
    if (busy || !mounted) return;
    setState(() => busy = true);
    try {
      final data = await widget.api.call('GET', '/api/v1/dashboard');
      final all = List<Json>.from(data['accounts'] as List? ?? []);
      final messages = await widget.api.call('GET', '/api/v1/notifications');
      final ruleData = await widget.api.call('GET', '/api/v1/notification-rules');
      if (mounted) { setState(() {
        accounts = all.where((a) => (a['provider'] as Json?)?['slug'] == 'cursor').toList();
        notifications = List<Json>.from(messages['notifications'] as List? ?? [])
            .where((n) => n['provider_slug'] == 'cursor').toList();
        rules = List<Json>.from(ruleData['rules'] as List? ?? []);
        error = null;
      }); }
    } catch (_) {
      if (mounted) setState(() => error = 'Cursor 数据暂不可用；已停止新的额度建议。');
    } finally { if (mounted) setState(() => busy = false); }
  }

  String remain(Json b) {
    final ratio=b['remaining_ratio'];
    if (b['collection_status']=='stale') return '缓存数据';
    if (ratio is num) return '${(ratio*100).round()}% 剩余';
    if (b['remaining_value'] is num) return '${b['remaining_value']} ${b['unit'] ?? ''} 剩余';
    return '不可用';
  }

  Iterable<Json> get buckets sync* {
    for (final a in accounts) {
      for (final b in List<Json>.from(a['buckets'] as List? ?? [])) {
        yield {...b, '_account': a['display_name']};
      }
    }
  }

  Widget overviewTab() => ListView(children: [
    if (accounts.isEmpty) const Card(child: Padding(padding: EdgeInsets.all(18), child: Text('还没有 Cursor 数据。请在电脑连接采集器或手工登记。'))),
    for (final b in buckets) Card(child: ListTile(
      title: Text('${b['_account']} · ${b['scope_key']}'),
      subtitle: Text('来源：${b['source_type']} · 更新：${b['observed_at'] ?? '未知'}'),
      trailing: Text(remain(b)),
    )),
  ]);

  Widget planTab() => ListView(children: [
    const Text('按实际采集的余量和重置时间规划；缺失数据不推断。'),
    for (final b in buckets) Card(child: Padding(padding: const EdgeInsets.all(16), child: Column(
      crossAxisAlignment: CrossAxisAlignment.start, children: [
        Text('${b['_account']} · ${b['scope_key']}', style: Theme.of(context).textTheme.titleMedium),
        Text(remain(b)),
        Text('重置：${b['reset_at'] ?? '时间未知'}'),
        Builder(builder: (_) {
          final reset=DateTime.tryParse(b['reset_at']?.toString() ?? '');
          final ratio=b['remaining_ratio'];
          final hours=reset?.difference(DateTime.now()).inMinutes;
          if (ratio is! num || hours==null || hours<=0 || b['collection_status']=='stale') return const Text('数据不足，暂不估算每日预算。');
          return Text('均匀使用参考：每天 ${(ratio/hours*144000).toStringAsFixed(1)} 个百分点。');
        }),
      ],
    ))),
  ]);

  Widget remindersTab() => ListView(children: [
    if (notifications.isEmpty) const Text('暂无 Cursor 提醒。'),
    for (final n in notifications) Card(child: ListTile(
      title: Text(n['title']?.toString() ?? ''),
      subtitle: Text(n['body']?.toString() ?? ''),
      trailing: n['status']=='unread' ? TextButton(onPressed: () async {
        await widget.api.call('POST', '/api/v1/notifications/${n['id']}/read'); await refresh();
      }, child: const Text('已读')) : const Text('已读'),
    )),
  ]);

  Widget radarTab() {
    final known=buckets.where((b)=>b['reset_at']!=null||b['expires_at']!=null).toList();
    return ListView(children: [
      const Text('仅展示 Cursor 实际返回的重置和到期时间。'),
      if (known.isEmpty) const Text('暂无可核实的重置时间。'),
      for (final b in known) Card(child: ListTile(title: Text('${b['_account']} · ${b['scope_key']}'),
        subtitle: Text('重置：${b['reset_at'] ?? '未知'}\n到期：${b['expires_at'] ?? '未知'}'))),
    ]);
  }

  Widget settingsTab() => ListView(children: [
    const Text('Cursor 提醒规则与当前账户同步。'),
    for (final rule in rules) SwitchListTile(
      title: Text(rule['name']?.toString() ?? '提醒规则'),
      subtitle: Text(rule['rule_type']?.toString() ?? ''),
      value: rule['enabled']==true,
      onChanged: (enabled) async {
        try {await widget.api.call('PATCH','/api/v1/notification-rules/${rule['id']}',{'enabled':enabled});await refresh();}
        catch (_) {if(mounted)setState(()=>error='规则保存失败');}
      },
    ),
  ]);

  @override
  Widget build(BuildContext context) => DefaultTabController(length: 5, child: Column(
    crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Text('Cursor 使用助手', style: Theme.of(context).textTheme.headlineMedium),
      const SizedBox(height: 5),
      const Text('额度、计划、提醒、雷达和设置'),
      if(error!=null) Text(error!,style:const TextStyle(color:HubTheme.warn)),
      TextButton(onPressed: busy ? null : refresh, child: const Text('刷新')),
      const TabBar(isScrollable:true,tabs:[
        Tab(text:'额度总览'),Tab(text:'使用计划'),Tab(text:'提醒中心'),Tab(text:'重置雷达'),Tab(text:'提醒设置')
      ]),
      Expanded(child:TabBarView(children:[overviewTab(),planTab(),remindersTab(),radarTab(),settingsTab()])),
    ],
  ));
}
