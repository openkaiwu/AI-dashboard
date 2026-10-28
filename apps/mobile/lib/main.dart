import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:drift/native.dart';
import 'local_notifications.dart';
import 'package:flutter/material.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';
import 'package:uuid/uuid.dart';
import 'api.dart';
import 'config.dart';
import 'store.dart';
import 'sync_engine.dart';
import 'codex_page.dart';
import 'cursor_page.dart';
import 'inbox_page.dart';
import 'theme.dart';
import 'appearance.dart';
import 'workspace_chrome.dart';
import 'manual_account.dart';
import 'strong_reminder_ui.dart';
import 'reminder_policy.dart';
import 'background_sync.dart';
import 'foreground_reminder_poller.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  unawaited(LocalNotifications.init());
  await appearance.load();
  runApp(const HubApp());
}

class HubApp extends StatelessWidget {
  const HubApp({super.key});
  @override
  Widget build(BuildContext context) => ListenableBuilder(
      listenable: appearance,
      builder: (context, _) => MaterialApp(
            title: 'AI Hub',
            debugShowCheckedModeBanner: false,
            theme: HubTheme.light(),
            darkTheme: HubTheme.dark(),
            themeMode: appearance.mode,
            home: const HubHome(),
          ));
}

class HubHome extends StatefulWidget {
  const HubHome({super.key});
  @override
  State<HubHome> createState() => _HubHomeState();
}

class _HubHomeState extends State<HubHome> with WidgetsBindingObserver {
  HubStore? store;
  HubApi? api;
  SyncEngine? engine;
  Json? selected;
  List<Json> profiles = [];
  LocalState state = LocalState();
  List<Json> devices = [];
  List<Json> bridges = [];
  Map<String, String> modeStatus = {};
  final vault = const FlutterSecureStorage();
  Timer? timer;
  late final ForegroundReminderPoller reminderPoller;
  bool reminderPopupOpen = false;
  String? error;
  bool busy = false;
  bool registerMode = false;
  bool registerDone = false;
  int tab = 0;
  String uiMode = 'choose';
  int manualRevision = 0;
  final email = TextEditingController(),
      password = TextEditingController(),
      deviceName = TextEditingController(text: '我的手机'),
      inviteCode = TextEditingController(),
      regNote = TextEditingController();

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    LocalNotifications.onAction = handleReminderAction;
    reminderPoller = ForegroundReminderPoller(
      check: () async {
        if (api?.session != null) {
          await pollStrongNotifications();
          if (mounted && error?.startsWith('提醒检查失败，将自动重试：') == true) {
            setState(() => error = null);
          }
        }
      },
      onError: (e) {
        if (mounted) setState(() => error = '提醒检查失败，将自动重试：$e');
      },
    );
    reminderPoller.setActive(
        WidgetsBinding.instance.lifecycleState == AppLifecycleState.resumed);
    unawaited(initialize());
    timer = Timer.periodic(const Duration(seconds: 30), (_) {
      if (engine != null && !busy) {
        unawaited(sync());
      }
    });
  }

  @override
  void dispose() {
    timer?.cancel();
    reminderPoller.dispose();
    WidgetsBinding.instance.removeObserver(this);
    email.dispose();
    password.dispose();
    deviceName.dispose();
    inviteCode.dispose();
    regNote.dispose();
    api?.client.close();
    unawaited(store?.close());
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState value) {
    reminderPoller.setActive(value == AppLifecycleState.resumed);
    if (value == AppLifecycleState.resumed && engine != null && !busy) {
      unawaited(sync());
    }
  }

  Future<void> initialize() async {
    try {
      final dir = await getApplicationSupportDirectory();
      store = HubStore(NativeDatabase.createInBackground(
          File(p.join(dir.path, 'aihub.sqlite'))));
      profiles = await store!.profiles();
      if (profiles.isEmpty) {
        final defaultProfile = {
          'id': AppConfig.defaultProfileId,
          'name': AppConfig.defaultProfileName,
          'url': AppConfig.defaultServerUrl,
        };
        await store!.saveProfile(defaultProfile);
        await store!.setActive(AppConfig.defaultProfileId);
        profiles = [defaultProfile];
      }
      final active = await store!.active();
      if (profiles.isNotEmpty) {
        await select(profiles.firstWhere((p) => p['id'] == active,
            orElse: () => profiles.first));
      }
      if (mounted) setState(() {});
    } catch (e) {
      if (mounted) setState(() => error = '初始化失败：$e');
    }
  }

  Future<void> select(Json profile) async {
    if (busy) return;
    api?.client.close();
    selected = profile;
    await store!.setActive(profile['id']);
    final raw = await vault.read(key: 'session:${profile['id']}');
    final session = raw == null ? null : jsonDecode(raw) as Json;
    api = HubApi(
        profile['url'],
        (s) =>
            vault.write(key: 'session:${profile['id']}', value: jsonEncode(s)),
        session: session);
    engine = api!.session == null
        ? null
        : SyncEngine(
            store!,
            '${profile['url']}|${api!.session!['user']['id']}|${api!.session!['device_id']}',
            api!);
    state = engine == null ? LocalState() : await engine!.state();
    devices = [];
    bridges = [];
    modeStatus = {};
    final savedMode = await vault.read(
        key:
            'uiMode:${profile['id']}:${session?['user']?['id']}:${session?['device_id']}');
    uiMode =
        savedMode == 'cursor' || savedMode == 'codex' ? savedMode! : 'choose';
    if (mounted) setState(() => error = null);
    if (engine != null) {
      unawaited(sync());
      unawaited(reminderPoller.checkNow());
      unawaited(refreshModeStatus());
    } else {
      unawaited(BackgroundSync.disable());
    }
  }

  Future<void> switchUiMode(String mode) async {
    setState(() => uiMode = mode);
    await vault.write(
        key:
            'uiMode:${selected?['id'] ?? 'default'}:${api?.session?['user']?['id']}:${api?.session?['device_id']}',
        value: mode);
    if (mode == 'choose') unawaited(refreshModeStatus());
  }

  void openManualAccount() {
    if (api?.session == null || (uiMode != 'codex' && uiMode != 'cursor')) {
      return;
    }
    showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      builder: (_) => ManualAccountSheet(
        api: api!,
        providerSlug: uiMode,
        onSaved: () {
          setState(() => manualRevision++);
          unawaited(refreshModeStatus());
        },
      ),
    );
  }

  Future<void> refreshModeStatus() async {
    final current = api;
    if (current?.session == null) return;
    final next = <String, String>{};
    try {
      final result = await current!.call('GET', '/api/v1/codex/overview');
      final snapshots = List<Json>.from(result['devices'] as List? ?? []);
      if (snapshots.isEmpty) {
        next['codex'] = '等待电脑连接';
      } else {
        final snapshot = snapshots.first['snapshot'] as Json?;
        final observed =
            DateTime.tryParse(snapshot?['observed_at']?.toString() ?? '');
        final fresh = observed != null &&
            DateTime.now().difference(observed) < const Duration(minutes: 11);
        next['codex'] = snapshot?['status'] == 'ok' && fresh
            ? '已连接 · 更新于 ${observed.toLocal().hour.toString().padLeft(2, '0')}:${observed.toLocal().minute.toString().padLeft(2, '0')}'
            : '等待电脑新数据';
      }
    } catch (_) {
      next['codex'] = '服务器暂不可用';
    }
    try {
      final result = await current!.call('GET', '/api/v1/dashboard');
      final accounts = List<Json>.from(result['accounts'] as List? ?? [])
          .where((a) => (a['provider'] as Json?)?['slug'] == 'cursor')
          .toList();
      next['cursor'] =
          accounts.isEmpty ? '等待 Cursor 数据' : '${accounts.length} 个账户 · 数据已同步';
    } catch (_) {
      next['cursor'] = '服务器暂不可用';
    }
    if (mounted && api == current) setState(() => modeStatus = next);
  }

  String? reminderScope(HubApi? current) => current?.session == null
      ? null
      : '${selected?['url']}|${current!.session!['user']['id']}|${current.session!['device_id']}';

  Future<void> reminderAction(HubApi current, String id, String action) async {
    final codex = id.startsWith('codex:');
    final serverId = codex ? Uri.decodeComponent(id.substring(6)) : id;
    final path = codex
        ? '/api/v1/codex/alerts/${Uri.encodeComponent(serverId)}'
        : '/api/v1/notifications/${Uri.encodeComponent(serverId)}/action';
    await current.call('POST', path, {'action': action});
    await LocalNotifications.cancelReminder(id);
  }

  Future<void> handleReminderAction(String id, String action) async {
    final current = api;
    final scope = reminderScope(current);
    if (current == null || scope == null) return;
    final seenKey = 'strong-seen:$scope:$id';
    if (action == 'open') {
      await pollStrongNotifications(requestedId: id);
      return;
    }
    if (!['read', 'snooze', 'dismiss'].contains(action)) return;
    try {
      await reminderAction(current, id, action);
      if (action == 'snooze' && !id.startsWith('codex:')) {
        await vault.delete(key: seenKey);
      }
    } catch (_) {
      if (mounted) setState(() => error = '提醒操作未保存，请在提醒中心重试。');
    }
  }

  Future<void> pollStrongNotifications({String? requestedId}) async {
    if (Platform.isAndroid) {
      if (api != null) await BackgroundSync.check(api!);
      return;
    }

    final current = api;
    final scope = reminderScope(current);
    if (current == null || scope == null || !mounted || reminderPopupOpen) {
      return;
    }
    reminderPopupOpen = true;
    try {
      Future<Json> genericNotifications() async {
        try {
          return await current.call('GET', '/api/v1/notifications');
        } catch (_) {
          return {'notifications': <Json>[]};
        }
      }

      final responses = await Future.wait([
        current.call('GET', '/api/v1/codex/overview'),
        genericNotifications(),
      ]);
      if (!mounted || api != current) return;
      final candidates = ReminderPolicy.candidates(
          responses[0],
          List<Json>.from(responses[1]['notifications'] as List? ?? []),
          DateTime.now());
      Json? next;
      for (final candidate in candidates) {
        if (requestedId != null && candidate['id'] != requestedId) continue;
        final delivered =
            await vault.read(key: 'strong-seen:$scope:${candidate['id']}');
        if (requestedId == null &&
            await BackgroundSync.seen(current, candidate['id'] as String,
                candidate['_token'] as String)) {
          continue;
        }
        if (requestedId != null || delivered != candidate['_token']) {
          next = candidate;
          break;
        }
      }
      if (!mounted || api != current) return;
      if (next == null) {
        if (requestedId != null) {
          setState(() => error = '这条提醒已处理、已失效或处于免打扰，请在提醒中心查看。');
        }
        return;
      }
      final id = next['id'] as String;
      final token = next['_token'] as String;
      final seenKey = 'strong-seen:$scope:$id';
      final cadenceKey =
          'reminder-cadence:${selected?['url']}|${current.session!['user']['id']}';
      Json? last;
      try {
        last = jsonDecode(await vault.read(key: cadenceKey) ?? 'null') as Json?;
      } catch (_) {}
      final priority = next['_priority'] as num;
      if (requestedId == null &&
          !ReminderPolicy.canInterrupt(last, priority, DateTime.now())) {
        return;
      }
      final isCodex = id.startsWith('codex:');
      final serverId = isCodex ? Uri.decodeComponent(id.substring(6)) : id;
      final deliveryPath =
          '/api/v1/codex/alerts/${Uri.encodeComponent(serverId)}/delivery';
      String? claim;
      if (isCodex && requestedId == null) {
        final result = await current
            .call('POST', deliveryPath, {'action': 'claim', 'token': token});
        if (result['claimed'] != true) {
          if (['delivered', 'changed', 'inactive'].contains(result['reason'])) {
            await vault.write(key: seenKey, value: token);
          }
          return;
        }
        claim = result['claim_id'] as String;
      }
      Future<void> receipt(String action) async {
        if (claim == null) return;
        try {
          await current.call(
              'POST', deliveryPath, {'action': action, 'claim_id': claim});
        } catch (_) {}
      }

      Future<void> recordCadence() async {
        try {
          await vault.write(
              key: cadenceKey,
              value: jsonEncode({
                'at': DateTime.now().millisecondsSinceEpoch,
                'priority': priority
              }));
        } catch (_) {}
      }

      if (!mounted) {
        await receipt('release');
        return;
      }
      if (api != current || reminderScope(current) != scope) {
        await receipt('release');
        return;
      }
      final foreground =
          WidgetsBinding.instance.lifecycleState == AppLifecycleState.resumed;
      if (!foreground) {
        final posted = await LocalNotifications.showReminder(
            id: id,
            title: StrongReminderCopy.notificationTitle(next),
            body: StrongReminderCopy.notificationBody(next));
        if (posted) {
          await receipt('delivered');
          await recordCadence();
          await vault.write(key: seenKey, value: token);
          await BackgroundSync.mark(current, id, token);
        } else {
          await receipt('release');
        }
        return;
      }
      final showing = showDialog<String>(
          context: context,
          barrierDismissible: false,
          builder: (ctx) => StrongReminderDialog(notification: next!));
      await receipt('delivered');
      await recordCadence();
      await vault.write(key: seenKey, value: token);
      await BackgroundSync.mark(current, id, token);
      final action = await showing;
      if (!mounted ||
          api != current ||
          reminderScope(current) != scope ||
          action == null) {
        return;
      }
      await reminderAction(current, id, action);
      if (!isCodex && action == 'snooze') {
        await vault.delete(key: seenKey);
      }
    } catch (_) {
      // Keep delivery markers after display; only unsent events may retry.
    } finally {
      reminderPopupOpen = false;
    }
  }

  Future<void> sync({bool rebuild = false}) async {
    if (busy || engine == null) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      if (rebuild) {
        await engine!.rebuild();
      } else {
        await engine!.sync();
      }
      state = await engine!.state();
    } catch (e) {
      error = '$e';
      state = await engine!.state();
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> signIn() async {
    if (api == null || busy) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      var installationId = await vault.read(key: 'installation_id');
      if (installationId == null) {
        installationId = const Uuid().v4();
        await vault.write(key: 'installation_id', value: installationId);
      }
      await api!.signIn(
          email.text.trim(), password.text, deviceName.text, installationId);
      password.clear();
      engine = SyncEngine(
          store!,
          '${selected!['url']}|${api!.session!['user']['id']}|${api!.session!['device_id']}',
          api!);
      state = await engine!.state();
      uiMode = 'choose';
      tab = 0;
    } catch (e) {
      if (e is ApiFailure && e.code == 'account_pending') {
        error = '账户正在等待管理员审核，批准后即可登录';
      } else {
        error = '$e';
      }
    } finally {
      if (mounted) setState(() => busy = false);
    }
    if (engine != null) {
      unawaited(sync());
      unawaited(reminderPoller.checkNow());
    }
  }

  /// Submits an invite-code registration; the server keeps the account
  /// pending until an administrator approves it.
  Future<void> register() async {
    if (api == null || busy) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      await api!.register(inviteCode.text.trim(), email.text.trim(),
          password.text, regNote.text.trim());
      setState(() => registerDone = true);
    } catch (e) {
      error = '$e';
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> editProfile([Json? existing]) async {
    final name = TextEditingController(
        text: existing?['name'] ?? AppConfig.defaultProfileName);
    final url = TextEditingController(
        text: existing?['url'] ?? AppConfig.defaultServerUrl);
    String? dialogError;
    final result = await showDialog<Json>(
        context: context,
        builder: (context) => StatefulBuilder(
            builder: (context, update) => AlertDialog(
                  title: Text(existing == null ? '添加服务器' : '编辑服务器'),
                  content: SizedBox(
                      width: 420,
                      child: Column(mainAxisSize: MainAxisSize.min, children: [
                        TextField(
                            controller: name,
                            decoration: const InputDecoration(labelText: '名称')),
                        const SizedBox(height: 16),
                        TextField(
                            controller: url,
                            decoration:
                                const InputDecoration(labelText: 'HTTPS 根地址')),
                        if (dialogError != null)
                          Text(dialogError!,
                              style: TextStyle(
                                  color: HubTheme.colors(context).warn)),
                        const Padding(
                            padding: EdgeInsets.only(top: 16),
                            child: Text('电脑和手机填写同一地址。公网连接必须使用 HTTPS。')),
                      ])),
                  actions: [
                    TextButton(
                        onPressed: () => Navigator.pop(context),
                        child: const Text('取消')),
                    FilledButton(
                        onPressed: () {
                          try {
                            final base = validateServer(url.text);
                            if (name.text.trim().isEmpty) {
                              throw ArgumentError('请填写名称');
                            }
                            Navigator.pop(context, {
                              'id': existing?['id'] ?? const Uuid().v4(),
                              'name': name.text.trim(),
                              'url': base
                            });
                          } catch (e) {
                            update(() => dialogError = '$e');
                          }
                        },
                        child: const Text('保存'))
                  ],
                )));
    name.dispose();
    url.dispose();
    if (result != null) {
      if (existing != null && existing['url'] != result['url']) {
        await vault.delete(key: 'session:${existing['id']}');
      }
      await store!.saveProfile(result);
      profiles = await store!.profiles();
      await select(result);
    }
  }

  Future<bool> confirm(String message) async =>
      await showDialog<bool>(
          context: context,
          builder: (context) => AlertDialog(
                  title: const Text('确认操作'),
                  content: Text(message),
                  actions: [
                    TextButton(
                        onPressed: () => Navigator.pop(context, false),
                        child: const Text('取消')),
                    FilledButton(
                        onPressed: () => Navigator.pop(context, true),
                        child: const Text('确认')),
                  ])) ??
      false;

  Future<void> compose([Json? note]) async {
    final title = TextEditingController(text: note?['payload']['title'] ?? '');
    final body = TextEditingController(text: note?['payload']['body'] ?? '');
    final saved = await showDialog<bool>(
        context: context,
        builder: (context) => AlertDialog(
                title: Text(note == null ? '新建便笺' : '编辑便笺'),
                content: SizedBox(
                    width: 440,
                    child: SingleChildScrollView(
                        child:
                            Column(mainAxisSize: MainAxisSize.min, children: [
                      TextField(
                          controller: title,
                          maxLength: 65,
                          decoration: const InputDecoration(labelText: '标题')),
                      const SizedBox(height: 12),
                      TextField(
                          controller: body,
                          maxLength: 4000,
                          minLines: 4,
                          maxLines: 8,
                          decoration: const InputDecoration(labelText: '内容')),
                    ]))),
                actions: [
                  TextButton(
                      onPressed: () => Navigator.pop(context, false),
                      child: const Text('取消')),
                  FilledButton(
                      onPressed: () {
                        if (title.text.trim().isNotEmpty) {
                          Navigator.pop(context, true);
                        }
                      },
                      child: const Text('保存到此设备'))
                ]));
    if (saved == true) {
      try {
        await engine!.save(title.text, body.text, id: note?['entity_id']);
        state = await engine!.state();
        setState(() {});
        unawaited(sync());
      } catch (e) {
        setState(() => error = '$e');
      }
    }
    title.dispose();
    body.dispose();
  }

  Future<void> loadDevices() async {
    try {
      final v = await api!.call('GET', '/api/v1/devices');
      final b = await api!.call('GET', '/api/v1/codex/bridges');
      setState(() {
        devices = List<Json>.from(v['devices']);
        bridges = List<Json>.from(b['bridges'] as List? ?? []);
      });
    } catch (e) {
      setState(() => error = '$e');
    }
  }

  Widget notes() {
    final visible = {...state.notes};
    for (final op in state.pending) {
      visible[op['entity_id']] = {
        'entity_id': op['entity_id'],
        'version': op['base_version'],
        'op': op['op'],
        'payload': op['payload']
      };
    }
    final items = visible.values.where((n) => n['op'] != 'delete').toList();
    return ListView(padding: const EdgeInsets.all(20), children: [
      Text('跨端便笺', style: Theme.of(context).textTheme.headlineSmall),
      const SizedBox(height: 8),
      const Text('先保存在此设备，联网后同步到你的服务器。'),
      const SizedBox(height: 20),
      Card(
          child: Padding(
              padding: const EdgeInsets.all(18),
              child: Wrap(spacing: 22, runSpacing: 12, children: [
                Text('便笺 ${items.length}'),
                Text('待上传 ${state.pending.length}'),
                Text('冲突 ${state.conflicts.length}'),
              ]))),
      Padding(
          padding: const EdgeInsets.symmetric(vertical: 12),
          child: Text(
              state.lastSync == null ? '尚未同步' : '上次成功同步：${state.lastSync}')),
      Wrap(spacing: 12, children: [
        FilledButton.icon(
            onPressed: busy ? null : () => sync(),
            icon: const Icon(Icons.sync),
            label: Text(busy ? '正在同步' : '立即同步')),
        OutlinedButton.icon(
            onPressed: () => compose(),
            icon: const Icon(Icons.add),
            label: const Text('新建便笺'))
      ]),
      const SizedBox(height: 16),
      for (final c in state.conflicts)
        Card(
            color: HubTheme.colors(context).warnSurface,
            child: Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      const Text('另一台设备已有更新'),
                      Text(
                          '本地：${c['operation']['payload']['title']}\n${c['operation']['payload']['body']}'),
                      Text(
                          '服务器：${c['current']['op'] == 'delete' ? '已删除' : c['current']['payload']['title']}\n${c['current']['payload']['body']}'),
                      Wrap(children: [
                        TextButton(
                            onPressed: () => resolve(c, false),
                            child: const Text('采用服务器版本')),
                        TextButton(
                            onPressed: () => resolve(c, true),
                            child: const Text('保留我的修改'))
                      ]),
                    ]))),
      if (items.isEmpty)
        const Padding(
            padding: EdgeInsets.symmetric(vertical: 56),
            child: Center(
                child: Text('从一条便笺开始\n在电脑记下，在手机接着看。',
                    textAlign: TextAlign.center))),
      for (final n in items)
        Card(
            child: Padding(
                padding: const EdgeInsets.all(18),
                child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                          blocked(n)
                              ? '待同步 / 有冲突'
                              : '已保存到服务器 · v${n['version']}',
                          style: Theme.of(context).textTheme.labelSmall),
                      const SizedBox(height: 12),
                      Text(n['payload']['title'],
                          style: Theme.of(context).textTheme.titleMedium),
                      const SizedBox(height: 8),
                      Text(n['payload']['body']),
                      const SizedBox(height: 8),
                      Wrap(children: [
                        TextButton(
                            onPressed: blocked(n) ? null : () => compose(n),
                            child: const Text('编辑')),
                        TextButton(
                            onPressed: blocked(n)
                                ? null
                                : () async {
                                    if (await confirm('删除便笺并同步到其他设备？')) {
                                      await engine!.save('', '',
                                          id: n['entity_id'], delete: true);
                                      state = await engine!.state();
                                      setState(() {});
                                      unawaited(sync());
                                    }
                                  },
                            child: const Text('删除'))
                      ]),
                    ]))),
      TextButton(
          onPressed: busy ? null : () => sync(rebuild: true),
          child: const Text('重新拉取服务器记录（保留待上传修改）')),
    ]);
  }

  bool blocked(Json n) =>
      state.pending.any((p) => p['entity_id'] == n['entity_id']) ||
      state.conflicts.any((c) => c['operation']['entity_id'] == n['entity_id']);

  Future<void> resolve(Json c, bool keep) async {
    await engine!.resolve(c['operation']['operation_id'], keep);
    state = await engine!.state();
    setState(() {});
    unawaited(sync());
  }

  Widget devicePage() => ListView(padding: const EdgeInsets.all(20), children: [
        Text('设备与连接', style: Theme.of(context).textTheme.headlineSmall),
        const SizedBox(height: 8),
        Text(selected?['url'] ?? ''),
        TextButton(onPressed: loadDevices, child: const Text('刷新设备')),
        for (final d in devices)
          Card(
              child: ListTile(
                  title: Text(
                      '${d['name']}${d['current'] == true ? ' · 此设备' : ''}'),
                  subtitle: Text(d['revoked_at'] == null ? '已授权' : '已撤销'),
                  trailing: Text(d['kind'] == 'desktop'
                      ? '电脑'
                      : d['kind'] == 'mobile'
                          ? '手机'
                          : '旧设备'))),
        const SizedBox(height: 20),
        Text('电脑采集器', style: Theme.of(context).textTheme.titleMedium),
        const Text('Codex 与 Cursor 由已绑定的电脑采集，手机只接收服务器数据。'),
        if (bridges.isEmpty) const ListTile(title: Text('尚无电脑采集连接')),
        for (final bridge in bridges)
          Card(
              child: ListTile(
            title: Text(bridge['name']?.toString() ?? '电脑'),
            subtitle: Text(bridge['received_at'] == null
                ? '等待首次上报'
                : '最近上报 ${bridge['received_at']}'),
            trailing: Text(bridge['revoked_at'] != null ? '已撤销' : '已授权'),
          )),
      ]);

  Widget serverPage() => ListView(padding: const EdgeInsets.all(20), children: [
        Text('你的服务器', style: Theme.of(context).textTheme.headlineSmall),
        const SizedBox(height: 16),
        for (final pr in profiles)
          Card(
              child: ListTile(
                  title: Text(pr['name']),
                  subtitle: Text(pr['url']),
                  onTap: busy ? null : () => select(pr),
                  trailing: Wrap(children: [
                    IconButton(
                        tooltip: '编辑',
                        onPressed: busy ? null : () => editProfile(pr),
                        icon: const Icon(Icons.edit_outlined)),
                    IconButton(
                        tooltip: '移除',
                        onPressed: busy
                            ? null
                            : () async {
                                if (await confirm('移除此服务器配置？本地便笺缓存会保留。')) {
                                  await store!.deleteProfile(pr['id']);
                                  await vault.delete(
                                      key: 'session:${pr['id']}');
                                  profiles = await store!.profiles();
                                  if (selected?['id'] == pr['id']) {
                                    selected = null;
                                    engine = null;
                                    api?.client.close();
                                    api = null;
                                    state = LocalState();
                                  }
                                  setState(() {});
                                }
                              },
                        icon: const Icon(Icons.delete_outline)),
                  ]))),
        FilledButton.icon(
            onPressed: busy ? null : () => editProfile(),
            icon: const Icon(Icons.add),
            label: const Text('添加服务器')),
        const SizedBox(height: 24),
        const Text('跨网络：手机与电脑登录同一 HTTPS 地址与账户即可同步。'),
        if (api?.session != null)
          TextButton(
              onPressed: busy
                  ? null
                  : () async {
                      try {
                        final pending =
                            state.pending.length + state.conflicts.length;
                        if (pending > 0) {
                          final discard = await showDialog<bool>(
                              context: context,
                              builder: (ctx) => AlertDialog(
                                      title: const Text('仍有本地修改'),
                                      content: Text(
                                          '有 $pending 条修改或冲突尚未处理。退出将清除本机副本。'),
                                      actions: [
                                        TextButton(
                                            onPressed: () =>
                                                Navigator.pop(ctx, false),
                                            child: const Text('返回')),
                                        TextButton(
                                            onPressed: () =>
                                                Navigator.pop(ctx, true),
                                            child: const Text('清除并退出'))
                                      ]));
                          if (discard != true) return;
                        }
                        final current = api!.session!;
                        final currentScope =
                            '${selected!['url']}|${current['user']['id']}|${current['device_id']}';
                        await api!.call('POST', '/api/v1/auth/logout');
                        await BackgroundSync.disable();
                        await store!.clearScope(currentScope);
                        for (final key in (await vault.readAll()).keys) {
                          if (key.startsWith('strong-seen:$currentScope:')) {
                            await vault.delete(key: key);
                          }
                        }
                        await vault.delete(key: 'session:${selected!['id']}');
                        api!.session = null;
                        engine = null;
                        state = LocalState();
                        uiMode = 'choose';
                        setState(() {});
                      } catch (e) {
                        setState(() => error = '$e');
                      }
                    },
              child: const Text('退出账户')),
      ]);

  Widget login() => ListView(padding: const EdgeInsets.all(20), children: [
        Container(
          padding: const EdgeInsets.all(22),
          decoration: BoxDecoration(
            color: HubTheme.colors(context).bg2,
            borderRadius: BorderRadius.circular(18),
            border: Border.all(color: HubTheme.colors(context).line),
          ),
          child:
              Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text('AI HUB / YOUR PERSONAL CONTROL ROOM',
                style: TextStyle(
                    fontSize: 11,
                    letterSpacing: 1.1,
                    color: HubTheme.colors(context).muted,
                    fontWeight: FontWeight.w600)),
            const SizedBox(height: 16),
            Text('连接你的工作空间', style: Theme.of(context).textTheme.headlineSmall),
            const SizedBox(height: 8),
            Text(
                '服务器：${selected?['name'] ?? '未选择'} · ${selected?['url'] ?? ''}'),
            const SizedBox(height: 20),
            if (selected == null)
              FilledButton(
                  onPressed: () => editProfile(), child: const Text('添加服务器'))
            else if (registerDone) ...[
              const Text('注册申请已提交，正在等待管理员审核。'),
              const SizedBox(height: 8),
              const Text('管理员批准后，使用邮箱和密码登录本服务器；若被拒绝请联系管理员。'),
              const SizedBox(height: 20),
              FilledButton(
                  onPressed: () => setState(() {
                        registerMode = false;
                        registerDone = false;
                        error = null;
                        password.clear();
                        inviteCode.clear();
                        regNote.clear();
                      }),
                  child: const Text('返回登录')),
            ] else if (registerMode) ...[
              TextField(
                  controller: inviteCode,
                  decoration: const InputDecoration(labelText: '邀请码')),
              const SizedBox(height: 16),
              TextField(
                  controller: email,
                  keyboardType: TextInputType.emailAddress,
                  decoration: const InputDecoration(labelText: '邮箱')),
              const SizedBox(height: 16),
              TextField(
                  controller: password,
                  obscureText: true,
                  decoration: const InputDecoration(labelText: '密码（至少 8 位）')),
              const SizedBox(height: 16),
              TextField(
                  controller: regNote,
                  decoration: const InputDecoration(labelText: '留言（可选）')),
              const SizedBox(height: 20),
              FilledButton(
                  onPressed: busy ? null : register,
                  child: const Text('提交注册申请')),
              TextButton(
                  onPressed: busy
                      ? null
                      : () => setState(() {
                            registerMode = false;
                            error = null;
                          }),
                  child: const Text('已有账户？返回登录')),
            ] else ...[
              TextField(
                  controller: email,
                  keyboardType: TextInputType.emailAddress,
                  decoration: const InputDecoration(labelText: '邮箱')),
              const SizedBox(height: 16),
              TextField(
                  controller: password,
                  obscureText: true,
                  decoration: const InputDecoration(labelText: '密码（至少 8 位）')),
              const SizedBox(height: 16),
              TextField(
                  controller: deviceName,
                  decoration: const InputDecoration(labelText: '此设备名称')),
              const SizedBox(height: 20),
              FilledButton(
                  onPressed: busy ? null : signIn,
                  child: const Text('进入工作空间 ↗')),
              TextButton(
                  onPressed: busy
                      ? null
                      : () => setState(() {
                            registerMode = true;
                            error = null;
                          }),
                  child: const Text('使用邀请码注册')),
              const Text('注册需管理员审核；换机请先联系管理员解绑。'),
            ],
          ]),
        ),
      ]);

  Widget pageBody() {
    if (tab == 4) return serverPage();
    if (engine == null) return login();
    if (tab == 1) {
      return InboxPage(
        key: ValueKey('inbox|${selected!['url']}|$uiMode'),
        api: api!,
        uiMode: uiMode,
        onModeSwitch: (mode) => unawaited(switchUiMode(mode)),
      );
    }
    if (tab == 0) {
      if (uiMode == 'choose') {
        return ListView(padding: const EdgeInsets.all(20), children: [
          Text('选择工作模式', style: Theme.of(context).textTheme.headlineMedium),
          const SizedBox(height: 10),
          const Text('查看 Codex 或 Cursor；后台采集与提醒不随页面切换停止。'),
          const SizedBox(height: 20),
          TextButton(onPressed: refreshModeStatus, child: const Text('刷新连接状态')),
          for (final mode in ['codex', 'cursor'])
            Card(
                child: Padding(
              padding: const EdgeInsets.all(20),
              child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(mode == 'codex' ? 'Codex' : 'Cursor',
                        style: Theme.of(context).textTheme.titleLarge),
                    const SizedBox(height: 8),
                    Text(mode == 'codex' ? '额度、使用计划、提醒与重置雷达' : '实际用量、提醒与重置时间'),
                    const SizedBox(height: 6),
                    Text(modeStatus[mode] ?? '正在检查连接状态',
                        style: Theme.of(context).textTheme.labelSmall),
                    const SizedBox(height: 12),
                    FilledButton(
                        onPressed: () => unawaited(switchUiMode(mode)),
                        child: Text(
                            '进入 ${mode == 'codex' ? 'Codex' : 'Cursor'} 模式')),
                  ]),
            )),
        ]);
      }
      if (uiMode == 'cursor') {
        return Padding(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 0),
          child: CursorPage(
              key: ValueKey(
                  'cursor|${selected!['url']}|${api!.session!['user']['id']}|${api!.session!['device_id']}|$manualRevision'),
              api: api!),
        );
      }
      return Padding(
        padding: const EdgeInsets.fromLTRB(20, 12, 20, 0),
        child: CodexPage(
          key: ValueKey(
              '${selected!['url']}|${api!.session!['user']['id']}|${api!.session!['device_id']}|$manualRevision'),
          api: api!,
          store: store!,
          onConnections: () {
            setState(() => tab = 3);
            unawaited(loadDevices());
          },
          scope:
              '${selected!['url']}|${api!.session!['user']['id']}|${api!.session!['device_id']}',
        ),
      );
    }
    if (tab == 2) return notes();
    return devicePage();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
        appBar: AppBar(
          title: HubBrand(
              subtitle: uiMode == 'cursor'
                  ? 'Cursor 工作空间'
                  : uiMode == 'codex'
                      ? 'Codex 工作空间'
                      : '选择工作模式'),
          actions: [
            const AppearanceMenu(),
            if (engine != null && tab == 0 && uiMode != 'choose')
              IconButton(
                tooltip: '手工登记额度',
                onPressed: openManualAccount,
                icon: const Icon(Icons.add_chart_outlined),
              ),
            if (engine != null)
              PopupMenuButton<String>(
                tooltip: '切换模式',
                initialValue: uiMode,
                onSelected: (v) => unawaited(switchUiMode(v)),
                itemBuilder: (context) => const [
                  PopupMenuItem(value: 'choose', child: Text('选择工作模式')),
                  PopupMenuItem(value: 'codex', child: Text('Codex 模式')),
                  PopupMenuItem(value: 'cursor', child: Text('Cursor 模式')),
                ],
                child: Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 8),
                  child: Container(
                    padding:
                        const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
                    decoration: BoxDecoration(
                        color: HubTheme.colors(context).bg2,
                        borderRadius: BorderRadius.circular(9)),
                    child: Row(mainAxisSize: MainAxisSize.min, children: [
                      Text(
                          uiMode == 'cursor'
                              ? 'Cursor'
                              : uiMode == 'codex'
                                  ? 'Codex'
                                  : '模式',
                          style: const TextStyle(
                              fontSize: 12, fontWeight: FontWeight.w600)),
                      const SizedBox(width: 3),
                      const Icon(Icons.expand_more, size: 16),
                    ]),
                  ),
                ),
              ),
          ],
        ),
        body: store == null
            ? Center(
                child: error == null
                    ? const CircularProgressIndicator()
                    : Text(error!))
            : Column(children: [
                if (error != null)
                  Container(
                    width: double.infinity,
                    color: HubTheme.colors(context).warnSurface,
                    padding: const EdgeInsets.all(14),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(error!),
                        if (engine != null)
                          TextButton(
                              onPressed: () {
                                setState(() {
                                  engine = null;
                                  api!.session = null;
                                });
                              },
                              child: const Text('重新登录（本地修改保留）')),
                      ],
                    ),
                  ),
                Expanded(
                    child:
                        SafeArea(top: false, bottom: false, child: pageBody())),
              ]),
        bottomNavigationBar: HubNavigationBar(
          selectedIndex: tab,
          onSelected: (v) {
            setState(() => tab = v);
            if (v == 3 && engine != null) unawaited(loadDevices());
          },
        ),
      );
}
