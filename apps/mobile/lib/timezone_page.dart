import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

class TimeZonePage extends StatefulWidget {
  const TimeZonePage({super.key});
  @override
  State<TimeZonePage> createState() => _TimeZonePageState();
}

class _TimeZonePageState extends State<TimeZonePage>
    with WidgetsBindingObserver {
  static const channel = MethodChannel('dev.aihub/timezone');
  Map<Object?, Object?> state = {};
  String error = '';
  bool busy = false;
  Timer? timer;
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    refresh();
    timer = Timer.periodic(const Duration(seconds: 2), (_) {
      final lifecycle = WidgetsBinding.instance.lifecycleState;
      if (lifecycle == null || lifecycle == AppLifecycleState.resumed) refresh();
    });
  }

  @override
  void dispose() {
    timer?.cancel();
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState value) {
    if (value == AppLifecycleState.resumed) refresh();
  }

  Future<void> refresh() async {
    try {
      final next = await channel.invokeMapMethod<Object?, Object?>('status');
      if (mounted) setState(() => state = next ?? {});
    } catch (e) {
      if (mounted) setState(() => error = '无法读取时区状态：$e');
    }
  }

  Future<void> invoke(String method, [Map<String, String>? args]) async {
    if (busy) return;
    setState(() {
      busy = true;
      error = '';
    });
    try {
      await channel.invokeMethod(method, args);
      await refresh();
    } on PlatformException catch (e) {
      if (mounted) setState(() => error = e.message ?? '操作失败，请重试。');
    } catch (e) {
      if (mounted) setState(() => error = '操作失败：$e');
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final ready = state['ready'] == true;
    final running = state['running'] == true;
    final zone = state['zone']?.toString() ?? '正在读取';
    return Scaffold(
        appBar: AppBar(title: const Text('时区切换')),
        body: ListView(padding: const EdgeInsets.all(20), children: [
          Text('当前手机时区', style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 8),
          Text(zone, style: Theme.of(context).textTheme.headlineSmall),
          Text(state['auto'] == null
              ? '正在读取时区设置'
              : state['auto'] == true
                  ? '自动时区已开启，切换时将关闭'
                  : '手动时区'),
          const SizedBox(height: 20),
          Card(
              child: ListTile(
                  leading: Icon(ready
                      ? Icons.check_circle_outline
                      : Icons.accessibility_new),
                  title: Text(ready ? '时区切换助手已就绪' : '需要开启时区切换助手'),
                  subtitle: const Text('助手仅在你发起切换后操作系统设置，最多运行 60 秒。'),
                  trailing: TextButton(
                      onPressed: busy ? null : () => invoke('settings'),
                      child: const Text('管理')))),
          const SizedBox(height: 16),
          for (final entry in const [
            ('la', '洛杉矶', '自动适配夏令时'),
            ('cn', '中国大陆', '北京时间 UTC+8')
          ])
            Card(
                child: ListTile(
                    title: Text(entry.$2),
                    subtitle: Text(
                        '${state[entry.$1 == 'la' ? 'laClock' : 'cnClock'] ?? '--:--:--'} · ${entry.$3}'),
                    trailing: FilledButton(
                        onPressed: busy || running
                            ? null
                            : () => invoke('switch', {'target': entry.$1}),
                        child: const Text('切换')))),
          if (running) const Text('正在操作系统设置，请保持手机解锁并等待完成。'),
          if ((state['message']?.toString() ?? '').isNotEmpty)
            Text(state['message'].toString()),
          if (error.isNotEmpty)
            Text(error,
                style: TextStyle(color: Theme.of(context).colorScheme.error)),
          const SizedBox(height: 20),
          const Text(
              '切换只关闭自动时区，保留自动校时。首次使用需在系统无障碍设置中开启“AI Hub 时区切换助手”；不同手机的设置界面可能影响自动操作。'),
          TextButton(
              onPressed: () => Clipboard.setData(ClipboardData(
                  text: state['diagnostic']?.toString() ?? '尚无诊断记录')),
              child: const Text('复制本机诊断')),
        ]));
  }
}
