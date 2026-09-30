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
import 'manual_account.dart';

void main() {
  unawaited(LocalNotifications.init());
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const HubApp());
}

class HubApp extends StatelessWidget {
  const HubApp({super.key});
  @override
  Widget build(BuildContext context) => MaterialApp(
        title: 'AI Hub',
        debugShowCheckedModeBanner: false,
        theme: HubTheme.light(),
        home: const HubHome(),
      );
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
  String? error;
  bool busy = false;
  int tab = 0;
  String uiMode = 'choose';
  int manualRevision = 0;
  final email = TextEditingController(),
      password = TextEditingController(),
      deviceName = TextEditingController(text: '我的手机');

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    unawaited(initialize());
    timer = Timer.periodic(const Duration(seconds: 30), (_) {
      if (engine != null && !busy) {
        unawaited(sync());
        unawaited(pollStrongNotifications());
      }
    });
  }

  @override
  void dispose() {
    timer?.cancel();
    WidgetsBinding.instance.removeObserver(this);
    email.dispose();
    password.dispose();
    deviceName.dispose();
    api?.client.close();
    unawaited(store?.close());
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState value) {
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
      unawaited(pollStrongNotifications());
      unawaited(refreshModeStatus());
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

  Future<void> pollStrongNotifications() async {
    if (api?.session == null || !mounted) return;
    try {
      final data = await api!.call('GET', '/api/v1/notifications');
      final items = List<Json>.from(data['notifications'] as List? ?? []);
      final next = items.cast<Json?>().firstWhere(
            (n) =>
                n != null &&
                n['status'] == 'unread' &&
                (n['severity'] == 'warning' || n['severity'] == 'critical'),
            orElse: () => null,
          );
      if (next == null || !mounted) return;
      final id = next['id'] as String;
      final scope =
          '${selected!['url']}|${api!.session!['user']['id']}|${api!.session!['device_id']}';
      final seenKey = 'strong-seen:$scope:$id';
      if (await vault.read(key: seenKey) != null) return;
      await LocalNotifications.showReminder(
        id: '$scope:$id',
        title: next['title'] as String? ?? 'AI Hub 提醒',
        body: next['body'] as String? ?? '',
      );
      final slug = next['provider_slug'] as String?;
      if (slug == 'cursor' || slug == 'codex') await switchUiMode(slug!);
      if (!mounted) return;
      final provider = next['provider_name'] as String? ?? 'AI Hub';
      final action = await showDialog<String>(
        context: context,
        barrierDismissible: false,
        builder: (ctx) => AlertDialog(
          title: Text('$provider · ${next['title']}'),
          content: Text(next['body'] as String? ?? ''),
          actions: [
            TextButton(
                onPressed: () => Navigator.pop(ctx, 'read'),
                child: const Text('已读')),
            TextButton(
                onPressed: () => Navigator.pop(ctx, 'later'),
                child: const Text('稍后')),
          ],
        ),
      );
      if (action == 'read') {
        await api!.call(
            'POST',
            '/api/v1/notifications/${Uri.encodeComponent(id)}/action',
            {'action': 'read'});
      }
      if (action == 'later') {
        await api!.call(
            'POST',
            '/api/v1/notifications/${Uri.encodeComponent(id)}/action',
            {'action': 'snooze'});
      }
      if (action == 'read') {
        await vault.write(key: seenKey, value: id);
      }
    } catch (_) {}
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
      error = '$e';
    } finally {
      if (mounted) setState(() => busy = false);
    }
    if (engine != null) unawaited(sync());
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
                              style: const TextStyle(color: HubTheme.warn)),
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
            color: const Color(0xfffff7e8),
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
            color: HubTheme.bg2,
            borderRadius: BorderRadius.circular(18),
            border: Border.all(color: HubTheme.line),
          ),
          child:
              Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            const Text('AI HUB / YOUR PERSONAL CONTROL ROOM',
                style: TextStyle(
                    fontSize: 11,
                    letterSpacing: 1.1,
                    color: HubTheme.muted,
                    fontWeight: FontWeight.w600)),
            const SizedBox(height: 16),
            Text('连接你的工作空间', style: Theme.of(context).textTheme.headlineSmall),
            const SizedBox(height: 8),
            Text(
                '服务器：${selected?['name'] ?? '未选择'} · ${selected?['url'] ?? ''}'),
            const SizedBox(height: 20),
            if (selected != null) ...[
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
              const Text('账户由管理员授权；换机请先联系管理员解绑。'),
            ] else
              FilledButton(
                  onPressed: () => editProfile(), child: const Text('添加服务器')),
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
          title: Row(children: [
            Container(
              width: 32,
              height: 32,
              decoration: BoxDecoration(
                color: HubTheme.accent,
                borderRadius: BorderRadius.circular(10),
              ),
              alignment: Alignment.center,
              child: const Text('↗',
                  style: TextStyle(color: Colors.white, fontSize: 18)),
            ),
            const SizedBox(width: 10),
            Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text('AI Hub', style: TextStyle(fontSize: 16)),
                Text(
                    uiMode == 'cursor'
                        ? 'Cursor 额度助手'
                        : uiMode == 'codex'
                            ? 'Codex 使用助手'
                            : '选择工作模式',
                    style:
                        const TextStyle(fontSize: 11, color: HubTheme.muted)),
              ],
            ),
          ]),
          actions: [
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
                  child: Text(uiMode == 'cursor' ? 'Cursor' : 'Codex',
                      style: Theme.of(context).textTheme.labelLarge),
                ),
              ),
            if (selected != null)
              Padding(
                  padding: const EdgeInsets.only(right: 16),
                  child: Center(
                      child: Text(selected!['name'],
                          style: Theme.of(context).textTheme.labelSmall))),
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
                    color: const Color(0xfffff7e8),
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
                Expanded(child: pageBody()),
              ]),
        bottomNavigationBar: NavigationBar(
            selectedIndex: tab,
            onDestinationSelected: (v) {
              setState(() => tab = v);
              if (v == 3 && engine != null) unawaited(loadDevices());
            },
            destinations: const [
              NavigationDestination(
                  icon: Icon(Icons.analytics_outlined), label: '额度'),
              NavigationDestination(
                  icon: Icon(Icons.notifications_outlined), label: '通知'),
              NavigationDestination(
                  icon: Icon(Icons.notes_outlined), label: '便笺'),
              NavigationDestination(
                  icon: Icon(Icons.devices_outlined), label: '设备'),
              NavigationDestination(
                  icon: Icon(Icons.dns_outlined), label: '服务器'),
            ]),
      );
}
