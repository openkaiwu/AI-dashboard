import 'dart:io';
import 'package:flutter/material.dart';
import 'api.dart';
import 'store.dart';
import 'background_sync.dart';
import 'local_notifications.dart';
import 'display_time.dart';
import 'theme.dart';
import 'notification_readiness.dart';

String notificationCondition(Object? value) =>
    const {
      'permission_denied': '未允许应用通知',
      'channel_missing': '通知渠道尚未创建',
      'channel_blocked': '额度通知渠道已关闭',
      'tray_only': '仅通知栏，横幅条件不足',
      'heads_up_conditions': '具备横幅条件，仍受系统设置影响',
      'system_dnd': '系统免打扰已开启，横幅或声音可能被抑制',
      'duplicate': '已提醒，不重复发送',
      'quiet_hours': '免打扰时段',
      'rate_limited': '30 分钟间隔内，暂不打扰',
      'no_new_events': '没有需要首次提醒的新事件',
      'no_source': '尚未收到电脑采集',
      'stale_source': '电脑采集数据过旧，请检查同步',
      'stale_response': '提醒响应已过期，请重试并检查手机日期和时间是否自动同步',
      'auth_expired': '后台授权失效，请重新启用',
      'not_primary': '当前不是主提醒手机',
      'notification_submitted': '已提交系统通知',
      'foreground_presented': '已在应用内展示提醒',
      'analysis_sync_failed': '分析缓存未更新，提醒独立检查',
      'network_or_delivery_error': '网络或送达检查失败，请查看诊断',
      'duplicate_or_inactive': '事件已提醒或已失效'
    }[value?.toString()] ??
    value?.toString() ??
    '尚未检查';

class BackgroundSettings extends StatefulWidget {
  final HubApi api;
  const BackgroundSettings({super.key, required this.api});
  @override
  State<BackgroundSettings> createState() => _BackgroundSettingsState();
}

class _BackgroundSettingsState extends State<BackgroundSettings>
    with WidgetsBindingObserver {
  Json info = {};
  bool busy = false;
  String? error;
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    reload();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) reload();
  }

  Future<void> reload() async {
    try {
      final next = await BackgroundSync.status();
      if (mounted) setState(() => info = next);
    } catch (e) {
      if (mounted) setState(() => error = '$e');
    }
  }

  Future<void> runAction(Future<void> Function() action) async {
    if (busy) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      await action();
    } catch (e) {
      if (mounted) setState(() => error = '操作未完成：$e');
    } finally {
      await reload();
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> toggle(bool value) => runAction(() async {
        if (value) {
          await LocalNotifications.requestPermission();
          await BackgroundSync.enable(widget.api);
          await BackgroundSync.check(widget.api);
        } else {
          await BackgroundSync.disable();
          await widget.api.call('DELETE', '/api/v1/mobile/background-token');
        }
      });

  Future<void> checkNow({bool makePrimary = false}) => runAction(() async {
        if (makePrimary) {
          await BackgroundSync.primary(widget.api);
        } else {
          await BackgroundSync.check(widget.api);
        }
      });

  Widget condition(String label, bool ready) => Container(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
        decoration: BoxDecoration(
            color: ready ? HubTheme.accentDim : HubTheme.bg2,
            borderRadius: BorderRadius.circular(8)),
        child: Row(mainAxisSize: MainAxisSize.min, children: [
          Icon(ready ? Icons.check_circle_outline : Icons.info_outline,
              size: 15, color: ready ? HubTheme.accent : HubTheme.warn),
          const SizedBox(width: 6),
          Text(label, style: const TextStyle(fontSize: 12)),
        ]),
      );

  Widget record(String label, Object? value) => Padding(
        padding: const EdgeInsets.only(bottom: 8),
        child: Text('$label：${formatLocalTimestamp(value)}',
            style: const TextStyle(fontSize: 12, color: HubTheme.muted)),
      );

  @override
  Widget build(BuildContext context) {
    if (!Platform.isAndroid) return const SizedBox.shrink();
    final enabled = info['enabled'] == true;
    final readiness = notificationReadiness(info);
    return Card(
        child: Padding(
      padding: const EdgeInsets.all(16),
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Row(children: [
          const Icon(Icons.notifications_active_outlined,
              color: HubTheme.accent, size: 21),
          const SizedBox(width: 10),
          Expanded(
              child: Text('这台手机的提醒',
                  style: Theme.of(context).textTheme.titleMedium)),
          if (busy)
            const SizedBox(
                width: 18,
                height: 18,
                child: CircularProgressIndicator(strokeWidth: 2)),
        ]),
        SwitchListTile(
            contentPadding: EdgeInsets.zero,
            title: const Text('自动同步并提醒'),
            subtitle: const Text('应用内每 15 秒检查；后台检查由系统安排。',
                style: TextStyle(fontSize: 12)),
            value: enabled,
            onChanged: busy || info.isEmpty ? null : toggle),
        if (info.isEmpty)
          const Text('正在读取此设备的提醒状态…')
        else
          Wrap(spacing: 8, runSpacing: 8, children: [
            condition(readiness.label, readiness.ready),
            condition(info['primary'] == true ? '主提醒手机' : '未确认为主手机',
                info['primary'] == true),
            condition(info['scheduled'] == true ? '后台已启用' : '后台未启用',
                info['scheduled'] == true),
          ]),
        const SizedBox(height: 14),
        Text(
            enabled
                ? notificationCondition(
                    info['last_reason'] ?? info['notification_state'])
                : '开启后，接收这台手机的额度与到期提醒。',
            style: const TextStyle(fontWeight: FontWeight.w600)),
        const SizedBox(height: 6),
        if (enabled) record('最近检查', info['last_sync']),
        if (enabled && info['primary'] != true)
          const Text('将本机设为主提醒手机后，才能接收真实事件提醒。',
              style: TextStyle(fontSize: 12, color: HubTheme.warn)),
        if (readiness.hint != null && info.isNotEmpty)
          Text(readiness.hint!,
              style: const TextStyle(fontSize: 12, color: HubTheme.warn)),
        const SizedBox(height: 10),
        Wrap(spacing: 8, runSpacing: 4, children: [
          FilledButton.icon(
              onPressed: busy || !enabled ? null : checkNow,
              icon: const Icon(Icons.refresh_rounded, size: 17),
              label: Text(busy ? '处理中…' : '立即检查提醒')),
          if (enabled && info['primary'] != true)
            OutlinedButton(
                onPressed: busy ? null : () => checkNow(makePrimary: true),
                child: const Text('设为主提醒手机')),
          TextButton(
              onPressed: busy
                  ? null
                  : () => runAction(() async {
                        await LocalNotifications.requestPermission();
                        await BackgroundSync.channel
                            .invokeMethod('notificationSettings');
                      }),
              child: const Text('通知与横幅设置')),
        ]),
        const SizedBox(height: 6),
        const Text('同一事件只提醒一次，30 分钟内合并提醒。应用打开时优先显示应用内提示。',
            style: TextStyle(fontSize: 12, color: HubTheme.muted)),
        if (error != null || info['error'] != null)
          Padding(
              padding: const EdgeInsets.only(top: 10),
              child: Text(error ?? info['error'].toString(),
                  style: const TextStyle(color: HubTheme.warn, fontSize: 12))),
        ExpansionTile(
          tilePadding: EdgeInsets.zero,
          title: const Text('后台运行与排查', style: TextStyle(fontSize: 13)),
          children: [
            Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              Text('系统通知：${notificationCondition(info['notification_state'])}',
                  style: const TextStyle(fontSize: 12)),
              const SizedBox(height: 8),
              Text(
                  info['battery_unrestricted'] == true
                      ? '电池：已允许不受优化限制。'
                      : '省电或休眠可能延迟后台检查。请允许后台运行；小米／HyperOS 还需检查自启动、悬浮通知与锁屏通知。',
                  style: const TextStyle(fontSize: 12, color: HubTheme.muted)),
              const SizedBox(height: 10),
              record('数据截至', info['source_observed_at']),
              record('后台最近执行', info['last_run']),
              record('最近提醒展示／系统提交', info['last_notification']),
              const Text('后台由系统约每 15 分钟安排。系统提交成功不代表已看到横幅，仍受免打扰与手机设置影响。',
                  style: TextStyle(fontSize: 12, color: HubTheme.muted)),
              Wrap(spacing: 8, children: [
                TextButton(
                    onPressed: busy
                        ? null
                        : () => runAction(() async {
                              await BackgroundSync.channel
                                  .invokeMethod('batterySettings');
                            }),
                    child: const Text('后台与电池设置')),
                TextButton(
                    onPressed: busy
                        ? null
                        : () => runAction(() async {
                              final ok = await LocalNotifications.showReminder(
                                  id: 'test',
                                  title: 'AI Hub 通知测试',
                                  body: '这是测试通知，不改变额度或真实提醒。');
                              if (mounted) {
                                setState(() => error = ok
                                    ? '已提交系统通知，请确认手机上是否出现横幅。'
                                    : '通知未发出，请检查权限与通知渠道。');
                              }
                            }),
                    child: const Text('测试系统通知')),
              ]),
              if (info['analysis_error'] != null)
                Text(info['analysis_error'].toString()),
              ExpansionTile(
                  tilePadding: EdgeInsets.zero,
                  title: const Text('最近诊断记录 · 保留 7 天',
                      style: TextStyle(fontSize: 12)),
                  children: [
                    SelectableText(info['diagnostics']?.toString() ?? '暂无记录')
                  ]),
              const Text('消耗记录保存在手机本地，覆盖安装会保留。电脑需开启同步，手机才能获取新数据。',
                  style: TextStyle(fontSize: 12, color: HubTheme.muted)),
            ])
          ],
        ),
      ]),
    ));
  }
}
