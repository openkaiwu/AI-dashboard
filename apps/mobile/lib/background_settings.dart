import 'dart:io';
import 'package:flutter/material.dart';
import 'api.dart';
import 'store.dart';
import 'background_sync.dart';
import 'local_notifications.dart';

String notificationCondition(Object? value) => const {
 'permission_denied':'未允许应用通知', 'channel_missing':'通知渠道尚未创建',
 'channel_blocked':'额度通知渠道已关闭', 'tray_only':'仅通知栏，横幅条件不足',
 'heads_up_conditions':'具备横幅条件，仍受系统设置影响',
 'duplicate':'已提醒，不重复发送', 'quiet_hours':'免打扰时段',
 'rate_limited':'30 分钟间隔内，暂不打扰', 'no_new_events':'没有需要首次提醒的新事件',
 'no_source':'尚未收到电脑采集', 'stale_source':'电脑采集数据过旧，请检查同步',
 'auth_expired':'后台授权失效，请重新启用', 'not_primary':'当前不是主提醒手机',
 'notification_submitted':'已提交系统通知', 'analysis_sync_failed':'分析缓存未更新，提醒独立检查',
 'network_or_delivery_error':'网络或送达检查失败，请查看诊断',
 'duplicate_or_inactive':'事件已提醒或已失效'
}[value?.toString()] ?? value?.toString() ?? '尚未检查';

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

  Future<void> toggle(bool value) async {
    setState(() {
      busy = true;
      error = null;
    });
    try {
      if (value) {
        await LocalNotifications.requestPermission();
        await BackgroundSync.enable(widget.api);
      } else {
        await BackgroundSync.disable();
        await widget.api.call('DELETE', '/api/v1/mobile/background-token');
      }
      await reload();
    } catch (e) {
      if (mounted) setState(() => error = '$e');
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (!Platform.isAndroid) return const SizedBox.shrink();
    return Card(
        child: Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Text('本地保存与后台通知',
                      style: Theme.of(context).textTheme.titleMedium),
                  const Text(
                      '消耗记录自动保存在手机本地。电脑需开启自动同步或点击立即同步，手机才能获取新数据。覆盖安装会保留记录；卸载时请选择保留应用数据。'),
                  SwitchListTile(
                      contentPadding: EdgeInsets.zero,
                      title: const Text('后台自动同步并提醒'),
                      subtitle: const Text('系统约每 15 分钟安排检查；省电、休眠和强制停止可能延迟。'),
                      value: info['enabled'] == true,
                      onChanged: busy ? null : toggle),
                  Text(
                      '通知权限：${info['notifications'] == true ? '已开启' : '未开启'} · 横幅渠道：${info['heads_up'] == true ? '具备横幅条件' : '请检查渠道'}'),
                  Text(
                      '主提醒手机：${info['primary'] == true ? '是' : '否或尚未检查'} · 后台任务：${info['scheduled'] == true ? '已注册' : '未启用'} · 电池：${info['battery_unrestricted'] == true ? '不受优化限制' : '可能受省电限制'}'),
                  const Text('同一事件只提醒一次；30 分钟内合并提醒。系统横幅仍受免打扰与手机设置影响。'),
                  Text('通知状态：${notificationCondition(info['notification_state'])}'),
                  if (info['source_observed_at'] != null)
                    Text('服务器最近收到的采集：${info['source_observed_at']}'),
                  if (info['last_run'] != null)
                    Text('后台最近执行：${info['last_run']}'),
                  if (info['last_notification'] != null)
                    Text('最近提交系统通知：${info['last_notification']}（不代表用户已看到横幅）'),
                  if (info['last_reason'] != null)
                    Text('最近检查结果：${notificationCondition(info['last_reason'])}'),
                  if (info['last_sync'] != null)
                    Text('最近提醒检查：${info['last_sync']}'),
                  Wrap(spacing: 8, children: [
                    TextButton(
                        onPressed: busy
                            ? null
                            : () async {
                                try {
                                  await BackgroundSync.primary(widget.api);
                                  await reload();
                                } catch (e) {
                                  if (mounted) setState(() => error = '$e');
                                }
                              },
                        child: const Text('设为主提醒手机')),
                    TextButton(
                        onPressed: () async {
                          await BackgroundSync.check(widget.api);
                          await reload();
                        },
                        child: const Text('检查真实提醒链路')),
                    TextButton(
                        onPressed: () async {
                          await LocalNotifications.requestPermission();
                          await BackgroundSync.channel
                              .invokeMethod('notificationSettings');
                        },
                        child: const Text('通知与横幅设置')),
                    TextButton(
                        onPressed: () => BackgroundSync.channel
                            .invokeMethod('batterySettings'),
                        child: const Text('后台运行与电池设置')),
                    TextButton(
                        onPressed: () async {
                          final ok = await LocalNotifications.showReminder(
                              id: 'test',
                              title: 'AI Hub 通知测试',
                              body: '这是测试通知，不改变额度或真实提醒。');
                          if (mounted) {
                            setState(() => error = ok
                                ? '已提交系统通知，请确认是否看到横幅；接口成功不代表横幅显示。'
                                : '通知未发出，请检查权限和通知渠道。');
                          }
                        },
                        child: const Text('测试本机系统通知')),
                  ]),
                  if (info['analysis_error'] != null)
                    Text(info['analysis_error'].toString()),
                  ExpansionTile(
                      title: const Text('最近诊断记录（本地保留 7 天）'),
                      children: [
                        SelectableText(
                            info['diagnostics']?.toString() ?? '暂无记录')
                      ]),
                  if (error != null || info['error'] != null)
                    Text(error ?? info['error'].toString(),
                        style: const TextStyle(color: Colors.red)),
                ])));
  }
}
